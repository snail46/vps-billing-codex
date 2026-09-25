package providerhealth

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	providercontract "vps-billing/backend/internal/provider"
)

const (
	checkInterval = 30 * time.Second
	checkTimeout  = 5 * time.Second
	batchSize     = 10
)

type Processor struct {
	pool      *pgxpool.Pool
	providers providercontract.Resolver
	now       func() time.Time
}

func New(pool *pgxpool.Pool, providers providercontract.Resolver) *Processor {
	return &Processor{pool: pool, providers: providers, now: time.Now}
}

func (p *Processor) ProcessBatch(ctx context.Context) (int, error) {
	now := p.now().UTC()
	rows, err := p.pool.Query(ctx, `SELECT id FROM providers WHERE status<>'disabled' AND (last_health_check_at IS NULL OR last_health_check_at<$1) ORDER BY last_health_check_at NULLS FIRST,id LIMIT $2`, now.Add(-checkInterval), batchSize)
	if err != nil {
		return 0, err
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if err = p.check(ctx, id, now); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

func (p *Processor) check(parent context.Context, providerID uuid.UUID, checkedAt time.Time) error {
	started := time.Now()
	ctx, cancel := context.WithTimeout(parent, checkTimeout)
	defer cancel()
	status, version, errorCode := "unavailable", "", ""
	capabilities := []byte(`{}`)
	details := []byte(`{}`)
	adapter, err := p.providers.Resolve(ctx, providerID)
	if err == nil {
		var health *providercontract.Health
		health, err = adapter.Health(ctx)
		if err == nil && health != nil {
			status, version = normalizeStatus(health.Status), health.Version
			if encoded, marshalErr := json.Marshal(health.Details); marshalErr == nil && len(encoded) > 0 {
				details = encoded
			}
			if value, capabilityErr := adapter.Capabilities(ctx); capabilityErr == nil && value != nil {
				capabilities, err = json.Marshal(value)
			} else if capabilityErr != nil {
				err = capabilityErr
			}
		}
	}
	if err != nil {
		errorCode = normalizedErrorCode(err)
		status = "unavailable"
	}
	latency := int(time.Since(started).Milliseconds())
	if latency < 0 {
		latency = 0
	}
	tx, err := p.pool.Begin(parent)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(parent, `INSERT INTO provider_health_checks(id,provider_id,status,version,latency_ms,capabilities,details,error_code,checked_at) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,NULLIF($8,''),$9)`, uuid.New(), providerID, status, version, latency, capabilities, details, errorCode, checkedAt)
	if err != nil {
		return err
	}
	providerStatus := status
	if providerStatus == "healthy" {
		providerStatus = "active"
	}
	_, err = tx.Exec(parent, `UPDATE providers SET status=$2,version=NULLIF($3,''),capabilities=CASE WHEN $4::jsonb='{}'::jsonb THEN capabilities ELSE $4::jsonb END,last_health_check_at=$5,updated_at=now() WHERE id=$1 AND status<>'disabled'`, providerID, providerStatus, version, capabilities, checkedAt)
	if err != nil {
		return err
	}
	return tx.Commit(parent)
}

func normalizeStatus(value string) string {
	switch value {
	case "healthy", "active", "online":
		return "healthy"
	case "degraded":
		return "degraded"
	default:
		return "unavailable"
	}
}

func normalizedErrorCode(err error) string {
	var providerErr *providercontract.Error
	if errors.As(err, &providerErr) {
		return providerErr.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return providercontract.ErrorTimeout
	}
	return providercontract.ErrorUnavailable
}
