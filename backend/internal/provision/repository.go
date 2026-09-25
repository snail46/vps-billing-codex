package provision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "vps-billing/backend/internal/store/sqlc"
)

var (
	ErrPurchaseNotReady = errors.New("purchase is not ready for provisioning")
	ErrPlacementMissing = errors.New("provision placement is missing")
)

var provisionSteps = []struct {
	key      string
	progress int32
}{
	{"validate_subscription", 5}, {"select_node", 15}, {"reserve_resources", 25},
	{"create_instance", 45}, {"wait_provider", 60}, {"configure_network", 70},
	{"verify_running", 80}, {"persist_network", 85}, {"commit_reservation", 90},
	{"activate_subscription", 95}, {"notify", 99},
}

type Repository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	now     func() time.Time
}

type Context struct {
	OperationID        uuid.UUID
	InstanceID         uuid.UUID
	InstanceName       string
	ProviderInstanceID string
	ObservedState      string
	SubscriptionID     uuid.UUID
	SubscriptionStatus string
	UserID             uuid.UUID
	SourceOrderID      uuid.UUID
	NodeGroupID        uuid.UUID
	CPUCores           float64
	MemoryMB           int64
	DiskGB             int64
	TrafficGB          *int64
	BandwidthMbps      *int
	IPv4Count          int32
	IPv6Count          int32
	NATPortCount       int32
	Virtualization     string
	ImageID            string
}

type Placement struct {
	ReservationID  uuid.UUID
	NodeID         uuid.UUID
	ProviderID     uuid.UUID
	ProviderNodeID string
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: db.New(pool), now: time.Now}
}

func (r *Repository) EnsureForPaidOrder(ctx context.Context, orderID uuid.UUID, traceID string) (int, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	purchase, err := queries.GetPaidPurchaseForProvision(ctx, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrPurchaseNotReady
	}
	if err != nil {
		return 0, err
	}
	if purchase.NodeGroupID == nil {
		return 0, fmt.Errorf("%w: plan has no node group", ErrPlacementMissing)
	}
	for index := int32(1); index <= purchase.Quantity; index++ {
		subscriptionID := deterministicID("subscription", orderID, index)
		instanceID := deterministicID("instance", orderID, index)
		operationID := deterministicID("operation", orderID, index)
		subscription, createErr := queries.CreatePurchaseSubscription(ctx, db.CreatePurchaseSubscriptionParams{
			ID: subscriptionID, UserID: purchase.UserID, PlanID: purchase.ID,
			BillingCycle: purchase.BillingCycle, PriceMinor: purchase.PriceMinor, Currency: purchase.Currency,
			SourceOrderID: &orderID, SourceItemIndex: pgtype.Int4{Int32: index, Valid: true},
		})
		if createErr != nil {
			return 0, createErr
		}
		instance, createErr := queries.CreatePendingInstance(ctx, db.CreatePendingInstanceParams{
			ID: instanceID, SubscriptionID: subscription.ID, Name: fmt.Sprintf("vps-%s-%d", orderID.String()[:8], index),
			CpuCores: purchase.CpuCores, MemoryMb: purchase.MemoryMb, DiskGb: purchase.DiskGb,
			TrafficLimitGb: purchase.TrafficGb, BandwidthMbps: purchase.BandwidthMbps,
			ImageID: pgtype.Text{String: purchase.DefaultImageID, Valid: true},
		})
		if createErr != nil {
			return 0, createErr
		}
		idempotencyKey := fmt.Sprintf("provision:%s:%d", orderID, index)
		operationRow, findErr := queries.GetOperationByIdempotency(ctx, idempotencyKey)
		if findErr == nil {
			continue
		}
		if !errors.Is(findErr, pgx.ErrNoRows) {
			return 0, findErr
		}
		operationRow, createErr = queries.CreateOperation(ctx, db.CreateOperationParams{
			ID: operationID, Type: "provision", ResourceType: "instance", ResourceID: instance.ID,
			MessageKey: text("operation.queued"), IdempotencyKey: idempotencyKey,
			MaxRetries: 5, TraceID: traceID, UserID: &purchase.UserID, Input: []byte("{}"),
		})
		if createErr != nil {
			return 0, createErr
		}
		for stepIndex, step := range provisionSteps {
			if _, createErr := queries.CreateOperationStep(ctx, db.CreateOperationStepParams{ID: deterministicID("step:"+step.key, orderID, index), OperationID: operationRow.ID, StepKey: step.key, StepOrder: int32(stepIndex + 1)}); createErr != nil {
				return 0, createErr
			}
		}
		if createErr := createOperationEvent(ctx, queries, operationRow); createErr != nil {
			return 0, createErr
		}
	}
	if err := queries.MarkOrderFulfilling(ctx, orderID); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int(purchase.Quantity), nil
}

func (r *Repository) Context(ctx context.Context, operationID uuid.UUID) (Context, error) {
	row, err := r.queries.GetProvisionContext(ctx, operationID)
	if err != nil {
		return Context{}, err
	}
	if row.NodeGroupID == nil || row.SourceOrderID == nil {
		return Context{}, ErrPlacementMissing
	}
	result := Context{OperationID: row.OperationID, InstanceID: row.InstanceID, InstanceName: row.InstanceName, ObservedState: row.ObservedState, SubscriptionID: row.SubscriptionID, SubscriptionStatus: row.SubscriptionStatus, UserID: row.UserID, SourceOrderID: *row.SourceOrderID, NodeGroupID: *row.NodeGroupID, CPUCores: row.CpuCores, MemoryMB: int64(row.MemoryMb), DiskGB: int64(row.DiskGb), IPv4Count: row.Ipv4Count, IPv6Count: row.Ipv6Count, NATPortCount: row.NatPortCount, Virtualization: row.Virtualization, ImageID: row.DefaultImageID}
	if row.ProviderInstanceID.Valid {
		result.ProviderInstanceID = row.ProviderInstanceID.String
	}
	if row.TrafficGb.Valid {
		value := row.TrafficGb.Int64
		result.TrafficGB = &value
	}
	if row.BandwidthMbps.Valid {
		value := int(row.BandwidthMbps.Int32)
		result.BandwidthMbps = &value
	}
	return result, nil
}

func (r *Repository) Placement(ctx context.Context, operationID uuid.UUID) (Placement, error) {
	row, err := r.queries.GetProvisionPlacement(ctx, operationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Placement{}, ErrPlacementMissing
	}
	if err != nil {
		return Placement{}, err
	}
	return Placement{ReservationID: row.ReservationID, NodeID: row.NodeID, ProviderID: row.ProviderID, ProviderNodeID: row.ProviderNodeID.String}, nil
}

func (r *Repository) SetPlacement(ctx context.Context, instanceID uuid.UUID, placement Placement) error {
	return r.queries.SetInstancePlacement(ctx, db.SetInstancePlacementParams{ID: instanceID, NodeID: &placement.NodeID, ProviderID: &placement.ProviderID})
}

func (r *Repository) SetProviderResult(ctx context.Context, instanceID uuid.UUID, providerInstanceID, state string, ipv4, ipv6 []string) error {
	return r.queries.SetInstanceProviderResult(ctx, db.SetInstanceProviderResultParams{ID: instanceID, ProviderInstanceID: text(providerInstanceID), ObservedState: state, PrimaryIpv4: firstAddress(ipv4), PrimaryIpv6: firstAddress(ipv6)})
}

func (r *Repository) SetError(ctx context.Context, instanceID uuid.UUID) error {
	return r.queries.SetInstanceProvisionError(ctx, instanceID)
}

func (r *Repository) Finalize(ctx context.Context, current Context, placement Placement) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	reservation, err := queries.LockResourceReservation(ctx, placement.ReservationID)
	if err != nil {
		return err
	}
	if reservation.Status == "reserved" {
		if _, err := queries.CommitNodeReservation(ctx, db.CommitNodeReservationParams{NodeID: reservation.NodeID, CpuCores: reservation.CpuCores, MemoryMb: reservation.MemoryMb, DiskGb: reservation.DiskGb, Ipv4Count: reservation.Ipv4Count, Ipv6Count: reservation.Ipv6Count, NatPortCount: reservation.NatPortCount}); err != nil {
			return err
		}
		if _, err := queries.SetResourceReservationStatus(ctx, db.SetResourceReservationStatusParams{ID: reservation.ID, Status: "committed"}); err != nil {
			return err
		}
	} else if reservation.Status != "committed" {
		return fmt.Errorf("reservation is %s", reservation.Status)
	}
	if current.SubscriptionStatus == "active" {
		if err := queries.MarkPurchaseOrderFulfilledIfReady(ctx, current.SourceOrderID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	activated, err := queries.ActivateProvisionedSubscription(ctx, db.ActivateProvisionedSubscriptionParams{ID: current.SubscriptionID, StartedAt: timestamp(r.now().UTC())})
	if err != nil {
		return err
	}
	if err := queries.MarkPurchaseOrderFulfilledIfReady(ctx, current.SourceOrderID); err != nil {
		return err
	}
	parameters, _ := json.Marshal(map[string]any{"instance_id": current.InstanceID})
	if err := queries.CreateProvisionNotification(ctx, db.CreateProvisionNotificationParams{ID: uuid.NewSHA1(uuid.NameSpaceURL, []byte("vps-billing:notification:"+current.InstanceID.String())), UserID: &current.UserID, Parameters: parameters}); err != nil {
		return err
	}
	if err := createDomainEvent(ctx, queries, "subscription.activated.v1", "subscription", activated.ID, map[string]any{"subscription_id": activated.ID, "user_id": activated.UserID, "instance_id": current.InstanceID, "status": activated.Status}); err != nil {
		return err
	}
	if err := createDomainEvent(ctx, queries, "instance.running.v1", "instance", current.InstanceID, map[string]any{"instance_id": current.InstanceID, "user_id": current.UserID, "subscription_id": current.SubscriptionID, "status": "running"}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func createOperationEvent(ctx context.Context, queries *db.Queries, row db.Operation) error {
	return createDomainEvent(ctx, queries, "operation.queued.v1", "operation", row.ID, map[string]any{"operation_id": row.ID, "user_id": row.UserID, "status": row.Status, "phase": row.Phase.String, "progress": row.Progress, "message_key": row.MessageKey.String, "retryable": row.Retryable, "error_code": row.ErrorCode.String, "trace_id": row.TraceID})
}

func createDomainEvent(ctx context.Context, queries *db.Queries, eventType, aggregateType string, aggregateID uuid.UUID, data map[string]any) error {
	eventID := newID()
	payload, err := json.Marshal(map[string]any{"event_id": eventID, "event_type": eventType, "occurred_at": time.Now().UTC(), "aggregate_type": aggregateType, "aggregate_id": aggregateID, "data": data})
	if err != nil {
		return err
	}
	return queries.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{ID: eventID, EventType: eventType, AggregateType: aggregateType, AggregateID: aggregateID, Payload: payload})
}

func deterministicID(kind string, orderID uuid.UUID, index int32) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("vps-billing:%s:%s:%d", kind, orderID, index)))
}

func newID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New()
	}
	return id
}

func text(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }
func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func firstAddress(values []string) *netip.Addr {
	for _, value := range values {
		if address, err := netip.ParseAddr(value); err == nil {
			return &address
		}
	}
	return nil
}
