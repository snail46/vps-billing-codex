-- name: CreateOperation :one
INSERT INTO operations (
  id, type, resource_type, resource_id, status, phase, progress, message_key,
  idempotency_key, retryable, retry_count, max_retries, trace_id, user_id, actor_admin_id,
  input, deadline_at, parent_operation_id, next_attempt_at
)
VALUES ($1, $2, $3, $4, 'queued', 'queued', 0, $5, $6, false, 0, $7, $8, $9, $10, $11, $12, $13, now())
RETURNING *;

-- name: CreateOperationStep :one
INSERT INTO operation_steps (id, operation_id, step_key, step_order, status, progress)
VALUES ($1, $2, $3, $4, 'pending', 0)
RETURNING *;

-- name: GetOperationByIdempotency :one
SELECT * FROM operations WHERE idempotency_key = $1;

-- name: GetOperationByID :one
SELECT * FROM operations WHERE id = $1;

-- name: GetOperationForUser :one
SELECT * FROM operations WHERE id = $1 AND user_id = $2;

-- name: ListOperationSteps :many
SELECT * FROM operation_steps WHERE operation_id = $1 ORDER BY step_order, id;

-- name: ClaimOperation :one
UPDATE operations
SET status = 'running', phase = COALESCE(phase, 'starting'), started_at = COALESCE(started_at, now()),
    heartbeat_at = now(), updated_at = now()
WHERE id = $1 AND status = 'queued' AND next_attempt_at <= now()
  AND (deadline_at IS NULL OR deadline_at > now())
RETURNING *;

-- name: UpdateOperationProgress :one
UPDATE operations
SET status = $2, phase = $3, progress = $4, message_key = $5, heartbeat_at = now(), updated_at = now()
WHERE id = $1 AND status NOT IN ('succeeded', 'failed', 'cancelled')
RETURNING *;

-- name: UpdateOperationStep :one
UPDATE operation_steps
SET status = sqlc.arg(status)::varchar, progress = sqlc.arg(progress), attempt = sqlc.arg(attempt),
    error_code = sqlc.arg(error_code), error_message = sqlc.arg(error_message), output = sqlc.arg(output),
    started_at = COALESCE(started_at, CASE WHEN sqlc.arg(status)::varchar = 'running' THEN now() END),
    finished_at = CASE WHEN sqlc.arg(status)::varchar IN ('succeeded', 'failed', 'skipped') THEN now() ELSE finished_at END,
    updated_at = now()
WHERE operation_id = sqlc.arg(operation_id) AND step_key = sqlc.arg(step_key)
RETURNING *;

-- name: CompleteOperation :one
UPDATE operations
SET status = $2, phase = $3, progress = $4, message_key = $5,
    retryable = false, error_code = $6, error_message = $7,
    heartbeat_at = now(), finished_at = now(), updated_at = now()
WHERE id = $1 AND status NOT IN ('succeeded', 'failed', 'cancelled')
RETURNING *;

-- name: ScheduleOperationRetry :one
UPDATE operations
SET status = 'retrying', phase = 'retrying', retryable = true,
    retry_count = retry_count + 1, error_code = $2, error_message = $3,
    message_key = $4, next_attempt_at = $5, heartbeat_at = now(), updated_at = now()
WHERE id = $1 AND status NOT IN ('succeeded', 'failed', 'cancelled')
RETURNING *;

-- name: ClaimDueOperationRetries :many
SELECT * FROM operations
WHERE status = 'retrying' AND next_attempt_at <= now()
ORDER BY next_attempt_at, created_at
FOR UPDATE SKIP LOCKED
LIMIT $1;

-- name: RequeueOperation :one
UPDATE operations
SET status = 'queued', phase = 'queued', progress = 0, message_key = 'operation.queued',
    retryable = false, heartbeat_at = NULL, updated_at = now()
WHERE id = $1 AND status = 'retrying'
RETURNING *;
