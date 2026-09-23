package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "vps-billing/backend/internal/store/sqlc"
)

const defaultLifecycleBatchSize int32 = 50

type PostgresRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, queries: db.New(pool)}
}

func (r *PostgresRepository) List(ctx context.Context, userID uuid.UUID) ([]Subscription, error) {
	rows, err := r.queries.ListSubscriptionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]Subscription, 0, len(rows))
	for _, row := range rows {
		result = append(result, fromListRow(row))
	}
	return result, nil
}

func (r *PostgresRepository) SetCancelAtPeriodEnd(ctx context.Context, userID, subscriptionID uuid.UUID, cancel bool) (Subscription, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Subscription{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	current, err := queries.LockSubscriptionByUser(ctx, db.LockSubscriptionByUserParams{ID: subscriptionID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	if err != nil {
		return Subscription{}, err
	}
	if current.Status != "active" {
		return Subscription{}, ErrInvalidState
	}
	if current.CancelAtPeriodEnd == cancel {
		return subscriptionView(ctx, queries, userID, subscriptionID)
	}
	updated, err := queries.SetSubscriptionCancelAtPeriodEnd(ctx, db.SetSubscriptionCancelAtPeriodEndParams{ID: subscriptionID, CancelAtPeriodEnd: cancel})
	if err != nil {
		return Subscription{}, err
	}
	eventType := "subscription.cancel_scheduled.v1"
	if !cancel {
		eventType = "subscription.cancel_schedule_removed.v1"
	}
	if err := createEvent(ctx, queries, eventType, updated, time.Now().UTC()); err != nil {
		return Subscription{}, err
	}
	result, err := subscriptionView(ctx, queries, userID, subscriptionID)
	if err != nil {
		return Subscription{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Subscription{}, err
	}
	return result, nil
}

type LifecycleProcessor struct {
	pool        *pgxpool.Pool
	gracePeriod time.Duration
	now         func() time.Time
	batchSize   int32
}

func NewLifecycleProcessor(pool *pgxpool.Pool, gracePeriod time.Duration) *LifecycleProcessor {
	return &LifecycleProcessor{pool: pool, gracePeriod: gracePeriod, now: time.Now, batchSize: defaultLifecycleBatchSize}
}

func (p *LifecycleProcessor) ProcessBatch(ctx context.Context) (int, error) {
	now := p.now().UTC()
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	rows, err := queries.ClaimSubscriptionsForLifecycle(ctx, db.ClaimSubscriptionsForLifecycleParams{CurrentPeriodEnd: timestamp(now), Limit: p.batchSize})
	if err != nil {
		return 0, err
	}
	for _, current := range rows {
		decision, ok := decideLifecycleTransition(current, now, p.gracePeriod)
		if !ok {
			continue
		}
		updated, updateErr := queries.UpdateSubscriptionLifecycle(ctx, db.UpdateSubscriptionLifecycleParams{ID: current.ID, Status: decision.status, GraceUntil: decision.graceUntil, EndedAt: decision.endedAt, Version: current.Version})
		if updateErr != nil {
			return 0, updateErr
		}
		if eventErr := createEvent(ctx, queries, decision.eventType, updated, now); eventErr != nil {
			return 0, eventErr
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(rows), nil
}

type lifecycleDecision struct {
	status     string
	eventType  string
	graceUntil pgtype.Timestamptz
	endedAt    pgtype.Timestamptz
}

func decideLifecycleTransition(current db.Subscription, now time.Time, gracePeriod time.Duration) (lifecycleDecision, bool) {
	switch {
	case current.Status == "active" && current.CancelAtPeriodEnd && current.CurrentPeriodEnd.Valid && !current.CurrentPeriodEnd.Time.After(now):
		return lifecycleDecision{status: "cancelled", eventType: "subscription.cancelled.v1", endedAt: timestamp(now)}, true
	case current.Status == "active" && current.CurrentPeriodEnd.Valid && !current.CurrentPeriodEnd.Time.After(now):
		return lifecycleDecision{status: "past_due", eventType: "subscription.past_due.v1", graceUntil: timestamp(current.CurrentPeriodEnd.Time.Add(gracePeriod))}, true
	case current.Status == "past_due" && current.GraceUntil.Valid && !current.GraceUntil.Time.After(now):
		return lifecycleDecision{status: "suspended", eventType: "subscription.suspended.v1", graceUntil: current.GraceUntil}, true
	default:
		return lifecycleDecision{}, false
	}
}

func createEvent(ctx context.Context, queries *db.Queries, eventType string, subscription db.Subscription, occurredAt time.Time) error {
	eventID := newID()
	payload, err := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": eventType, "occurred_at": occurredAt,
		"aggregate_type": "subscription", "aggregate_id": subscription.ID,
		"data": map[string]any{"subscription_id": subscription.ID, "user_id": subscription.UserID, "status": subscription.Status, "version": subscription.Version},
	})
	if err != nil {
		return err
	}
	return queries.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{ID: eventID, EventType: eventType, AggregateType: "subscription", AggregateID: subscription.ID, Payload: payload})
}

func fromListRow(row db.ListSubscriptionsByUserRow) Subscription {
	return Subscription{ID: row.ID, PlanID: row.PlanID, PlanSlug: row.PlanSlug, PlanName: row.PlanNameI18n, Status: row.Status, BillingCycle: row.BillingCycle, PriceMinor: row.PriceMinor, Currency: row.Currency, StartedAt: timePointer(row.StartedAt), CurrentPeriodStart: timePointer(row.CurrentPeriodStart), CurrentPeriodEnd: timePointer(row.CurrentPeriodEnd), NextDueAt: timePointer(row.NextDueAt), GraceUntil: timePointer(row.GraceUntil), CancelAtPeriodEnd: row.CancelAtPeriodEnd, EndedAt: timePointer(row.EndedAt), Version: row.Version}
}

func subscriptionView(ctx context.Context, queries *db.Queries, userID, subscriptionID uuid.UUID) (Subscription, error) {
	rows, err := queries.ListSubscriptionsByUser(ctx, userID)
	if err != nil {
		return Subscription{}, err
	}
	for _, row := range rows {
		if row.ID == subscriptionID {
			return fromListRow(row), nil
		}
	}
	return Subscription{}, ErrNotFound
}

func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func newID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New()
	}
	return id
}
