package operation

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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
	row, err := queries.CreateOperation(ctx, db.CreateOperationParams{ID: operationID, Type: request.Type, ResourceType: request.ResourceType, ResourceID: request.ResourceID, MessageKey: text("operation.queued"), IdempotencyKey: request.IdempotencyKey, MaxRetries: request.MaxRetries, TraceID: request.TraceID, UserID: request.UserID, ActorAdminID: request.ActorAdminID, Input: nonNilJSON(request.Input), DeadlineAt: optionalTimestamp(request.DeadlineAt), ParentOperationID: request.ParentID})
	if err != nil {
		if isUniqueViolation(err) {
			_ = tx.Rollback(ctx)
			existing, lookupErr := r.queries.GetOperationByIdempotency(ctx, request.IdempotencyKey)
			if errors.Is(lookupErr, pgx.ErrNoRows) {
				return Operation{}, ErrIdempotencyConflict
			}
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
	if _, err := tx.Exec(ctx, `INSERT INTO operation_attempts(id,operation_id,attempt,status) VALUES($1,$2,$3,'running') ON CONFLICT(operation_id,attempt) DO NOTHING`, newID(), row.ID, row.RetryCount+1); err != nil {
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
	if _, err := tx.Exec(ctx, `UPDATE operation_attempts SET status='retrying',error_code=$3,error_message=$4,finished_at=now() WHERE operation_id=$1 AND attempt=$2 AND finished_at IS NULL`, current.ID, current.RetryCount+1, code, raw); err != nil {
		return err
	}
	if err := createEvent(ctx, queries, "operation.updated.v1", updated, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RecoverStuck requeues operations whose owning worker stopped heartbeating.
// PostgreSQL and the transactional outbox are the recovery source; a fresh
// queue event means recovery does not depend on the original Redis message.
func (r *PostgresRepository) RecoverStuck(ctx context.Context, cutoff time.Time, batchSize int) (int, error) {
	if batchSize <= 0 {
		return 0, ErrInvalidRequest
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT id FROM operations
		WHERE status IN ('running','waiting_provider','waiting_resource','verifying') AND heartbeat_at IS NOT NULL AND heartbeat_at < $1
		ORDER BY heartbeat_at FOR UPDATE SKIP LOCKED LIMIT $2`, cutoff.UTC(), batchSize)
	if err != nil {
		return 0, err
	}
	var ids []uuid.UUID
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
	queries := db.New(tx)
	for _, id := range ids {
		result, updateErr := tx.Exec(ctx, `UPDATE operations SET
			status=CASE WHEN retry_count < max_retries THEN 'queued' ELSE 'failed' END,
			phase=CASE WHEN retry_count < max_retries THEN 'queued' ELSE 'failed' END,
			progress=CASE WHEN retry_count < max_retries THEN 0 ELSE 100 END,
			message_key=CASE WHEN retry_count < max_retries THEN 'operation.queued' ELSE 'operation.failed' END,
			retryable=retry_count < max_retries,
			retry_count=CASE WHEN retry_count < max_retries THEN retry_count+1 ELSE retry_count END,
			error_code='WORKER_HEARTBEAT_EXPIRED',error_message='worker heartbeat expired',
			heartbeat_at=NULL,finished_at=CASE WHEN retry_count < max_retries THEN NULL ELSE now() END,updated_at=now()
			WHERE id=$1 AND status IN ('running','waiting_provider','waiting_resource','verifying')`, id)
		if updateErr != nil {
			return 0, updateErr
		}
		if result.RowsAffected() == 0 {
			continue
		}
		updated, getErr := queries.GetOperationByID(ctx, id)
		if getErr != nil {
			return 0, getErr
		}
		eventType := "operation.updated.v1"
		if updated.Status == "queued" {
			eventType = "operation.queued.v1"
		}
		if eventErr := createEvent(ctx, queries, eventType, updated, map[string]any{"recovery": "worker_heartbeat_expired"}); eventErr != nil {
			return 0, eventErr
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (r *PostgresRepository) CreateAdminRetry(ctx context.Context, sourceID, adminID uuid.UUID, idempotencyKey, traceID string) (Operation, error) {
	if existing, err := r.queries.GetOperationByIdempotency(ctx, idempotencyKey); err == nil {
		if existing.ParentOperationID == nil || *existing.ParentOperationID != sourceID || existing.ActorAdminID == nil || *existing.ActorAdminID != adminID {
			return Operation{}, ErrIdempotencyConflict
		}
		return r.operationWithSteps(ctx, existing)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Operation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT id FROM operations WHERE id=$1 FOR UPDATE`, sourceID); err != nil {
		return Operation{}, err
	}
	queries := db.New(tx)
	source, err := queries.GetOperationByID(ctx, sourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrNotFound
	}
	if err != nil {
		return Operation{}, err
	}
	if source.Status != "failed" && source.Status != "cancelled" {
		return Operation{}, ErrStateConflict
	}
	retryID := newID()
	deadline := pgtype.Timestamptz{}
	if source.DeadlineAt.Valid {
		deadline = timestamp(r.now().UTC().Add(10 * time.Minute))
	}
	created, err := queries.CreateOperation(ctx, db.CreateOperationParams{ID: retryID, Type: source.Type, ResourceType: source.ResourceType, ResourceID: source.ResourceID, MessageKey: text("operation.queued"), IdempotencyKey: idempotencyKey, MaxRetries: source.MaxRetries, TraceID: traceID, UserID: source.UserID, ActorAdminID: &adminID, Input: source.Input, DeadlineAt: deadline, ParentOperationID: &sourceID})
	if err != nil {
		if isUniqueViolation(err) {
			return Operation{}, ErrIdempotencyConflict
		}
		return Operation{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO operation_steps(id,operation_id,step_key,step_order,status,progress) SELECT gen_random_uuid(),$1,step_key,step_order,'pending',0 FROM operation_steps WHERE operation_id=$2 ORDER BY step_order`, retryID, sourceID); err != nil {
		return Operation{}, err
	}
	if err = createEvent(ctx, queries, "operation.queued.v1", created, map[string]any{"parent_operation_id": sourceID}); err != nil {
		return Operation{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(id,actor_type,actor_id,action,resource_type,resource_id,before_data,after_data,trace_id) VALUES($1,'admin',$2,'operation.retried','operation',$3,jsonb_build_object('status',$4),jsonb_build_object('retry_operation_id',$5),$6)`, newID(), adminID, sourceID, source.Status, retryID, traceID); err != nil {
		return Operation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Operation{}, err
	}
	return r.operationWithSteps(ctx, created)
}

func (r *PostgresRepository) CancelAdmin(ctx context.Context, operationID, adminID uuid.UUID, traceID string) (Operation, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Operation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	var previous string
	if err = tx.QueryRow(ctx, `SELECT status FROM operations WHERE id=$1 FOR UPDATE`, operationID).Scan(&previous); errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrNotFound
	} else if err != nil {
		return Operation{}, err
	}
	if previous != "queued" && previous != "retrying" {
		return Operation{}, ErrStateConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE operations SET status='cancelled',phase='cancelled',progress=100,message_key='operation.cancelled',retryable=false,error_code='ADMIN_CANCELLED',error_message='cancelled by administrator',finished_at=now(),updated_at=now() WHERE id=$1`, operationID); err != nil {
		return Operation{}, err
	}
	updated, err := queries.GetOperationByID(ctx, operationID)
	if err != nil {
		return Operation{}, err
	}
	if err = createEvent(ctx, queries, "operation.updated.v1", updated, nil); err != nil {
		return Operation{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(id,actor_type,actor_id,action,resource_type,resource_id,before_data,after_data,trace_id) VALUES($1,'admin',$2,'operation.cancelled','operation',$3,jsonb_build_object('status',$4),jsonb_build_object('status','cancelled'),$5)`, newID(), adminID, operationID, previous, traceID); err != nil {
		return Operation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Operation{}, err
	}
	return r.operationWithSteps(ctx, updated)
}

func (r *PostgresRepository) ExpireDeadlines(ctx context.Context, now time.Time, batchSize int) (int, error) {
	if batchSize <= 0 {
		return 0, ErrInvalidRequest
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT id FROM operations WHERE deadline_at<=$1 AND status IN ('queued','retrying') ORDER BY deadline_at FOR UPDATE SKIP LOCKED LIMIT $2`, now.UTC(), batchSize)
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
	queries := db.New(tx)
	for _, id := range ids {
		if _, err = tx.Exec(ctx, `UPDATE operations SET status='failed',phase='failed',progress=100,message_key='operation.failed',retryable=false,error_code='OPERATION_DEADLINE_EXCEEDED',error_message='operation deadline exceeded',finished_at=now(),updated_at=now() WHERE id=$1`, id); err != nil {
			return 0, err
		}
		updated, getErr := queries.GetOperationByID(ctx, id)
		if getErr != nil {
			return 0, getErr
		}
		if getErr = createEvent(ctx, queries, "operation.updated.v1", updated, nil); getErr != nil {
			return 0, getErr
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(ids), nil
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
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	updated, err := queries.CompleteOperation(ctx, db.CompleteOperationParams{ID: operationID, Status: status, Phase: text(phase), Progress: progress, MessageKey: text(messageKey), ErrorCode: text(errorCode), ErrorMessage: text(errorMessage)})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE operation_attempts SET status=$3,error_code=NULLIF($4,''),error_message=NULLIF($5,''),finished_at=now() WHERE operation_id=$1 AND attempt=$2 AND finished_at IS NULL`, operationID, updated.RetryCount+1, status, errorCode, errorMessage); err != nil {
		return err
	}
	if err := createEvent(ctx, queries, "operation.updated.v1", updated, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
	attemptRows, err := r.pool.Query(ctx, `SELECT attempt,status,error_code,started_at,finished_at FROM operation_attempts WHERE operation_id=$1 ORDER BY attempt`, row.ID)
	if err != nil {
		return Operation{}, err
	}
	defer attemptRows.Close()
	result.Attempts = []Attempt{}
	for attemptRows.Next() {
		var item Attempt
		var code pgtype.Text
		var finished pgtype.Timestamptz
		if err := attemptRows.Scan(&item.Attempt, &item.Status, &code, &item.StartedAt, &finished); err != nil {
			return Operation{}, err
		}
		item.ErrorCode, item.FinishedAt = textPointer(code), timePointer(finished)
		item.StartedAt = item.StartedAt.UTC()
		result.Attempts = append(result.Attempts, item)
	}
	if err := attemptRows.Err(); err != nil {
		return Operation{}, err
	}
	return result, nil
}

func publicOperation(row db.Operation) Operation {
	return Operation{ID: row.ID, Type: row.Type, ResourceType: row.ResourceType, ResourceID: row.ResourceID, Status: row.Status, Phase: textPointer(row.Phase), Progress: row.Progress, MessageKey: textPointer(row.MessageKey), Retryable: row.Retryable, RetryCount: row.RetryCount, MaxRetries: row.MaxRetries, ErrorCode: textPointer(row.ErrorCode), TraceID: row.TraceID, StartedAt: timePointer(row.StartedAt), FinishedAt: timePointer(row.FinishedAt), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}
}

func sameRequest(row db.Operation, request CreateRequest) bool {
	return row.Type == request.Type && row.ResourceType == request.ResourceType && row.ResourceID == request.ResourceID && equalUUID(row.UserID, request.UserID) && equalUUID(row.ActorAdminID, request.ActorAdminID) && equalUUID(row.ParentOperationID, request.ParentID) && equalJSON(row.Input, nonNilJSON(request.Input))
}
func equalUUID(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
func equalJSON(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
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
func optionalTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(*value)
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
