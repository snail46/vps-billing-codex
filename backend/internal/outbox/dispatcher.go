package outbox

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	db "vps-billing/backend/internal/store/sqlc"
)

type Dispatcher struct {
	pool      *pgxpool.Pool
	redis     *redis.Client
	stream    string
	batchSize int32
}

func NewDispatcher(pool *pgxpool.Pool, redisClient *redis.Client) *Dispatcher {
	return &Dispatcher{pool: pool, redis: redisClient, stream: "domain-events", batchSize: 50}
}

func (d *Dispatcher) ProcessBatch(ctx context.Context) (int, error) {
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	events, err := queries.ClaimOutboxEvents(ctx, d.batchSize)
	if err != nil {
		return 0, err
	}
	for _, event := range events {
		if _, err := d.redis.XAdd(ctx, &redis.XAddArgs{Stream: d.stream, Values: map[string]any{"event_id": event.ID.String(), "event_type": event.EventType, "payload": string(event.Payload)}}).Result(); err != nil {
			if retryErr := queries.MarkOutboxRetry(ctx, event.ID); retryErr != nil {
				return 0, fmt.Errorf("publish outbox: %w; record retry: %v", err, retryErr)
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return 0, commitErr
			}
			return 0, fmt.Errorf("publish outbox event %s: %w", event.ID, err)
		}
		if err := queries.MarkOutboxPublished(ctx, event.ID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(events), nil
}
