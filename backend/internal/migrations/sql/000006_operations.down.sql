DROP INDEX IF EXISTS ix_operations_user;
DROP INDEX IF EXISTS ix_operations_dispatch;

ALTER TABLE operation_steps
  DROP CONSTRAINT IF EXISTS operation_steps_status_valid,
  DROP COLUMN IF EXISTS output;

ALTER TABLE operations
  DROP CONSTRAINT IF EXISTS operations_status_valid,
  DROP COLUMN IF EXISTS heartbeat_at,
  DROP COLUMN IF EXISTS next_attempt_at,
  DROP COLUMN IF EXISTS actor_admin_id,
  DROP COLUMN IF EXISTS user_id;
