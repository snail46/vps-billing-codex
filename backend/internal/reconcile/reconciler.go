package reconcile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vps-billing/backend/internal/operation"
	providercontract "vps-billing/backend/internal/provider"
)

const (
	defaultBatchSize       = 25
	defaultOperationMaxAge = 2 * time.Minute
	defaultHeartbeatMaxAge = 90 * time.Second
	providerReadTimeout    = 5 * time.Second
)

type operationCreator interface {
	Create(context.Context, operation.CreateRequest) (operation.Operation, error)
}

// Processor repairs durable state after process, Redis, provider, or agent
// failures. Every mutation is idempotent and PostgreSQL remains authoritative.
type Processor struct {
	pool                *pgxpool.Pool
	providers           providercontract.Resolver
	operations          operationCreator
	operationRepository *operation.PostgresRepository
	now                 func() time.Time
	batchSize           int
}

func New(pool *pgxpool.Pool, providers providercontract.Resolver, operations operationCreator, operationRepository *operation.PostgresRepository) *Processor {
	return &Processor{pool: pool, providers: providers, operations: operations, operationRepository: operationRepository, now: time.Now, batchSize: defaultBatchSize}
}

func (p *Processor) ProcessBatch(ctx context.Context) (int, error) {
	now := p.now().UTC()
	total, err := p.operationRepository.RecoverStuck(ctx, now.Add(-defaultOperationMaxAge), p.batchSize)
	if err != nil {
		return total, fmt.Errorf("recover stuck operations: %w", err)
	}
	count, err := p.expireReservations(ctx, now)
	total += count
	if err != nil {
		return total, fmt.Errorf("expire reservations: %w", err)
	}
	count, err = p.expireNodeHeartbeats(ctx, now.Add(-defaultHeartbeatMaxAge))
	total += count
	if err != nil {
		return total, fmt.Errorf("expire node heartbeats: %w", err)
	}
	count, err = p.reconcileInstances(ctx)
	total += count
	if err != nil {
		return total, fmt.Errorf("reconcile instances: %w", err)
	}
	return total, nil
}

func (p *Processor) expireReservations(ctx context.Context, now time.Time) (int, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT r.id,r.node_id,r.cpu_cores,r.memory_mb,r.disk_gb,r.ipv4_count,r.ipv6_count,r.nat_port_count
		FROM resource_reservations r JOIN operations o ON o.id=r.operation_id
		WHERE r.status='reserved' AND r.expires_at<=$1 AND o.status IN ('succeeded','failed','cancelled')
		ORDER BY r.expires_at FOR UPDATE OF r SKIP LOCKED LIMIT $2`, now, p.batchSize)
	if err != nil {
		return 0, err
	}
	type reservation struct {
		id, nodeID      uuid.UUID
		cpu             float64
		memory, disk    int64
		ipv4, ipv6, nat int32
	}
	var items []reservation
	for rows.Next() {
		var item reservation
		if err = rows.Scan(&item.id, &item.nodeID, &item.cpu, &item.memory, &item.disk, &item.ipv4, &item.ipv6, &item.nat); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, item := range items {
		result, updateErr := tx.Exec(ctx, `UPDATE nodes SET cpu_reserved=cpu_reserved-$2,memory_reserved_mb=memory_reserved_mb-$3,disk_reserved_gb=disk_reserved_gb-$4,ipv4_reserved=ipv4_reserved-$5,ipv6_reserved=ipv6_reserved-$6,nat_port_reserved=nat_port_reserved-$7,version=version+1,updated_at=now() WHERE id=$1`, item.nodeID, item.cpu, item.memory, item.disk, item.ipv4, item.ipv6, item.nat)
		if updateErr != nil {
			return 0, updateErr
		}
		if result.RowsAffected() != 1 {
			return 0, errors.New("reservation node is missing")
		}
		if _, updateErr = tx.Exec(ctx, `UPDATE resource_reservations SET status='expired',updated_at=now() WHERE id=$1 AND status='reserved'`, item.id); updateErr != nil {
			return 0, updateErr
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(items), nil
}

func (p *Processor) expireNodeHeartbeats(ctx context.Context, cutoff time.Time) (int, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `UPDATE nodes n SET status='offline',updated_at=now()
		FROM agent_connections c WHERE c.node_id=n.id AND n.status IN ('online','degraded')
		AND COALESCE(c.last_heartbeat_at,c.connected_at)<$1 RETURNING n.id`, cutoff)
	if err != nil {
		return 0, err
	}
	var nodeIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		nodeIDs = append(nodeIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, nodeID := range nodeIDs {
		if _, err = tx.Exec(ctx, `UPDATE agent_connections SET status='disconnected',disconnected_at=COALESCE(disconnected_at,now()) WHERE node_id=$1`, nodeID); err != nil {
			return 0, err
		}
		if _, err = tx.Exec(ctx, `UPDATE instances SET observed_state='unknown',last_synced_at=now(),version=version+1,updated_at=now() WHERE node_id=$1 AND observed_state NOT IN ('deleted','unknown')`, nodeID); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(nodeIDs), nil
}

type instanceCandidate struct {
	id, providerID, userID uuid.UUID
	providerInstanceID     string
	providerNodeID         string
	desired, observed      string
}

func (p *Processor) reconcileInstances(ctx context.Context) (int, error) {
	if _, err := p.pool.Exec(ctx, `UPDATE instances i SET desired_state='suspended',version=i.version+1,updated_at=now()
		FROM subscriptions s WHERE s.id=i.subscription_id AND s.status='suspended' AND i.desired_state NOT IN ('suspended','deleted')`); err != nil {
		return 0, err
	}
	if _, err := p.pool.Exec(ctx, `UPDATE instances i SET desired_state='running',version=i.version+1,updated_at=now()
		FROM subscriptions s WHERE s.id=i.subscription_id AND s.status='active' AND i.desired_state='suspended'`); err != nil {
		return 0, err
	}
	rows, err := p.pool.Query(ctx, `SELECT i.id,i.provider_id,i.provider_instance_id,COALESCE(n.provider_node_id,n.id::text),i.desired_state,i.observed_state,s.user_id
		FROM instances i JOIN nodes n ON n.id=i.node_id JOIN subscriptions s ON s.id=i.subscription_id
		WHERE i.deleted_at IS NULL AND i.provider_id IS NOT NULL AND i.provider_instance_id IS NOT NULL AND n.status='online'
		ORDER BY COALESCE(i.last_synced_at,'epoch'::timestamptz),i.id LIMIT $1`, p.batchSize)
	if err != nil {
		return 0, err
	}
	var items []instanceCandidate
	for rows.Next() {
		var item instanceCandidate
		if err = rows.Scan(&item.id, &item.providerID, &item.providerInstanceID, &item.providerNodeID, &item.desired, &item.observed, &item.userID); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, item := range items {
		adapter, resolveErr := p.providers.Resolve(ctx, item.providerID)
		if resolveErr != nil {
			continue
		}
		providerCtx, cancel := context.WithTimeout(ctx, providerReadTimeout)
		observed, getErr := adapter.GetInstance(providerCtx, providercontract.GetInstanceRequest{NodeID: item.providerNodeID, ProviderInstanceID: item.providerInstanceID, PlatformInstanceID: item.id.String()})
		cancel()
		if getErr != nil {
			var providerErr *providercontract.Error
			if errors.As(getErr, &providerErr) && (providerErr.Code == providercontract.ErrorNodeOffline || providerErr.Code == providercontract.ErrorUnavailable || providerErr.Code == providercontract.ErrorTimeout) {
				if _, updateErr := p.pool.Exec(ctx, `UPDATE instances SET observed_state='unknown',last_synced_at=now(),version=version+1,updated_at=now() WHERE id=$1 AND observed_state<>'unknown'`, item.id); updateErr != nil {
					return processed, updateErr
				}
			}
			continue
		}
		state := observed.State
		if state == "deleted" {
			state = "unknown"
		}
		var version int64
		if err = p.pool.QueryRow(ctx, `UPDATE instances SET provider_instance_id=$2,observed_state=$3::varchar,last_synced_at=now(),version=version+CASE WHEN observed_state<>$3::varchar THEN 1 ELSE 0 END,updated_at=now() WHERE id=$1 RETURNING version`, item.id, observed.ProviderInstanceID, state).Scan(&version); err != nil {
			return processed, err
		}
		processed++
		if state == item.desired || (item.desired == "suspended" && state == "suspended") {
			continue
		}
		action := ""
		switch {
		case item.desired == "running" && (state == "stopped" || state == "suspended"):
			action = "start"
		case item.desired == "stopped" && state == "running":
			action = "stop"
		case item.desired == "suspended" && (state == "running" || state == "stopped"):
			action = "suspend"
		}
		if action == "" {
			continue
		}
		_, createErr := p.operations.Create(ctx, operation.CreateRequest{Type: action, ResourceType: "instance", ResourceID: item.id, IdempotencyKey: fmt.Sprintf("reconcile:%s:%s:%d", item.id, item.desired, version), TraceID: "reconciler:" + uuid.NewString(), UserID: &item.userID, MaxRetries: 3, Steps: []operation.StepDefinition{{Key: "validate", Order: 1}, {Key: "provider", Order: 2}, {Key: "verify", Order: 3}}})
		if createErr != nil && !errors.Is(createErr, operation.ErrIdempotencyConflict) {
			return processed, createErr
		}
	}
	return processed, nil
}
