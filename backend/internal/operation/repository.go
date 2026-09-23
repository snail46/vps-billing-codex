package operation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "vps-billing/backend/internal/store/sqlc"
)

type PostgresRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	now     func() time.Time
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, queries: db.New(pool), now: time.Now}
}

func (r *PostgresRepository) Create(ctx context.Context, request CreateRequest) (Operation, error) {
	if existing, err := r.queries.GetOperationByIdempotency(ctx, request.IdempotencyKey); err == nil {
		if !sameRequest(existing, request) {
			return Operation{}, ErrIdempotencyConflict
		}
		return r.operationWithSteps(ctx, existing)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, err
	}
	for _, definition := range request.Steps {
		if definition.Key == "" {
			return Operation{}, ErrInvalidRequest
		}
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Operation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	operationID := newID()
	row, err := queries.CreateOperation(ctx, db.CreateOperationParams{ID: operationID, Type: request.Type, ResourceType: request.ResourceType, ResourceID: request.ResourceID, MessageKey: text("operation.queued"), IdempotencyKey: request.IdempotencyKey, MaxRetries: request.MaxRetries, TraceID: request.TraceID, UserID: request.UserID, ActorAdminID: request.ActorAdminID})
	if err != nil {
		if isUniqueViolation(err) {
			_ = tx.Rollback(ctx)
			existing, lookupErr := r.queries.GetOperationByIdempotency(ctx, request.IdempotencyKey)
			if lookupErr != nil {
				return Operation{}, lookupErr
			}
			if !sameRequest(existing, request) {
				return Operation{}, ErrIdempotencyConflict
			}
			return r.operationWithSteps(ctx, existing)
		}
		return Operation{}, err
	}
	for _, definition := range request.Steps {
		if _, err := queries.CreateOperationStep(ctx, db.CreateOperationStepParams{ID: newID(), OperationID: operationID, StepKey: definition.Key, StepOrder: definition.Order}); err != nil {
			return Operation{}, err
		}
	}
	if err := createEvent(ctx, queries, "operation.queued.v1", row, nil); err != nil {
		return Operation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Operation{}, err
	}
	return r.operationWithSteps(ctx, row)
}

func (r *PostgresRepository) GetForUser(ctx context.Context, userID, operationID uuid.UUID) (Operation, error) {
	row, err := r.queries.GetOperationForUser(ctx, db.GetOperationForUserParams{ID: operationID, UserID: &userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrNotFound
	}
	if err != nil {
		return Operation{}, err
	}
	return r.operationWithSteps(ctx, row)
}

func (r *PostgresRepository) Get(ctx context.Context, operationID uuid.UUID) (db.Operation, error) {
	row, err := r.queries.GetOperationByID(ctx, operationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Operation{}, ErrNotFound
	}
	return row, err
}

func (r *PostgresRepository) Claim(ctx context.Context, operationID uuid.UUID) (db.Operation, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return db.Operation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	row, err := queries.ClaimOperation(ctx, operationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Operation{}, ErrNotFound
	}
	if err != nil {
		return db.Operation{}, err
	}
	if err := createEvent(ctx, queries, "operation.updated.v1", row, nil); err != nil {
		return db.Operation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Operation{}, err
	}
	return row, nil
}

func (r *PostgresRepository) UpdateProgress(ctx context.Context, operationID uuid.UUID, status, phase string, progress int32, messageKey string) error {
	if progress < 0 || progress > 100 {
		return ErrInvalidRequest
	}
	return r.mutate(ctx, operationID, func(queries *db.Queries) (db.Operation, error) {
		return queries.UpdateOperationProgress(ctx, db.UpdateOperationProgressParams{ID: operationID, Status: status, Phase: text(phase), Progress: progress, MessageKey: text(messageKey)})
	})
}

func (r *PostgresRepository) UpdateStep(ctx context.Context, operationID uuid.UUID, stepKey, status string, progress, attempt int32, errorCode, errorMessage string, output json.RawMessage) error {
	if progress < 0 || progress > 100 {
		return ErrInvalidRequest
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	step, err := queries.UpdateOperationStep(ctx, db.UpdateOperationStepParams{OperationID: operationID, Status: status, Progress: progress, Attempt: attempt, ErrorCode: text(errorCode), ErrorMessage: text(errorMessage), Output: nonNilJSON(output), StepKey: stepKey})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	operationRow, err := queries.GetOperationByID(ctx, operationID)
	if err != nil {
		return err
	}
	if err := createEvent(ctx, queries, "operation.updated.v1", operationRow, map[string]any{"step_key": step.StepKey, "step_status": step.Status, "step_progress": step.Progress}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) Succeed(ctx context.Context, operationID uuid.UUID) error {
	return r.complete(ctx, operationID, "succeeded", "finished", 100, "operation.succeeded", "", "")
}

func (r *PostgresRepository) Fail(ctx context.Context, operationID uuid.UUID, workflowErr *WorkflowError) error {
	code, messageKey, raw := "WORKFLOW_FAILED", "operation.failed", ""
	if workflowErr != nil {
		code, messageKey = workflowErr.Code, workflowErr.MessageKey
		if workflowErr.Cause != nil {
			raw = workflowErr.Cause.Error()
		}
	}
	return r.complete(ctx, operationID, "failed", "failed", 100, messageKey, code, raw)
}

func (r *PostgresRepository) ScheduleRetry(ctx context.Context, current db.Operation, workflowErr *WorkflowError, next time.Time) error {
	code, messageKey, raw := "WORKFLOW_RETRY", "operation.retrying", ""
	if workflowErr != nil {
		code, messageKey = workflowErr.Code, workflowErr.MessageKey
		if workflowErr.Cause != nil {
			raw = workflowErr.Cause.Error()
		}
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	updated, err := queries.ScheduleOperationRetry(ctx, db.ScheduleOperationRetryParams{ID: current.ID, ErrorCode: text(code), ErrorMessage: text(raw), MessageKey: text(messageKey), NextAttemptAt: timestamp(next)})
	if err != nil {
		return err
	}
	if err := createEvent(ctx, queries, "operation.updated.v1", updated, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) mutate(ctx context.Context, operationID uuid.UUID, mutation func(*db.Queries) (db.Operation, error)) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	updated, err := mutation(queries)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := createEvent(ctx, queries, "operation.updated.v1", updated, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) complete(ctx context.Context, operationID uuid.UUID, status, phase string, progress int32, messageKey, errorCode, errorMessage string) error {
	return r.mutate(ctx, operationID, func(queries *db.Queries) (db.Operation, error) {
		return queries.CompleteOperation(ctx, db.CompleteOperationParams{ID: operationID, Status: status, Phase: text(phase), Progress: progress, MessageKey: text(messageKey), ErrorCode: text(errorCode), ErrorMessage: text(errorMessage)})
	})
}

func (r *PostgresRepository) operationWithSteps(ctx context.Context, row db.Operation) (Operation, error) {
	rows, err := r.queries.ListOperationSteps(ctx, row.ID)
	if err != nil {
		return Operation{}, err
	}
	result := publicOperation(row)
	result.Steps = make([]Step, 0, len(rows))
	for _, step := range rows {
		result.Steps = append(result.Steps, Step{Key: step.StepKey, Order: step.StepOrder, Status: step.Status, Progress: step.Progress, Attempt: step.Attempt, ErrorCode: textPointer(step.ErrorCode), Output: step.Output, StartedAt: timePointer(step.StartedAt), FinishedAt: timePointer(step.FinishedAt)})
	}
	return result, nil
}

func publicOperation(row db.Operation) Operation {
	return Operation{ID: row.ID, Type: row.Type, ResourceType: row.ResourceType, ResourceID: row.ResourceID, Status: row.Status, Phase: textPointer(row.Phase), Progress: row.Progress, MessageKey: textPointer(row.MessageKey), Retryable: row.Retryable, RetryCount: row.RetryCount, MaxRetries: row.MaxRetries, ErrorCode: textPointer(row.ErrorCode), TraceID: row.TraceID, StartedAt: timePointer(row.StartedAt), FinishedAt: timePointer(row.FinishedAt), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}
}

func sameRequest(row db.Operation, request CreateRequest) bool {
	return row.Type == request.Type && row.ResourceType == request.ResourceType && row.ResourceID == request.ResourceID && equalUUID(row.UserID, request.UserID) && equalUUID(row.ActorAdminID, request.ActorAdminID)
}
func equalUUID(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func createEvent(ctx context.Context, queries *db.Queries, eventType string, row db.Operation, extra map[string]any) error {
	eventID := newID()
	data := map[string]any{"operation_id": row.ID, "user_id": row.UserID, "status": row.Status, "phase": row.Phase.String, "progress": row.Progress, "message_key": row.MessageKey.String, "retryable": row.Retryable, "error_code": row.ErrorCode.String, "trace_id": row.TraceID}
	for key, value := range extra {
		data[key] = value
	}
	payload, err := json.Marshal(map[string]any{"event_id": eventID, "event_type": eventType, "occurred_at": time.Now().UTC(), "aggregate_type": "operation", "aggregate_id": row.ID, "data": data})
	if err != nil {
		return err
	}
	return queries.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{ID: eventID, EventType: eventType, AggregateType: "operation", AggregateID: row.ID, Payload: payload})
}

func text(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }
func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
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
func nonNilJSON(value json.RawMessage) []byte {
	if len(value) == 0 {
		return []byte(`{}`)
	}
	return value
}
func newID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New()
	}
	return id
}

func isUniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}
