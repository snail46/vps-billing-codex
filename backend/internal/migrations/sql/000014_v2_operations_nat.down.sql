DROP INDEX IF EXISTS ux_operations_active_port_forward_add;
DROP INDEX IF EXISTS ix_port_forwards_instance_active;
DROP INDEX IF EXISTS ux_port_forwards_operation;
DROP INDEX IF EXISTS ux_port_forward_provider_mapping;
DROP INDEX IF EXISTS ux_port_forward_active;

ALTER TABLE port_forwards
  DROP CONSTRAINT IF EXISTS port_forwards_status_valid,
  DROP COLUMN IF EXISTS error_code,
  DROP COLUMN IF EXISTS operation_id;

CREATE UNIQUE INDEX ux_port_forward ON port_forwards(public_ip, protocol, public_port);

DROP INDEX IF EXISTS ix_operations_deadline;
DROP TABLE IF EXISTS operation_attempts;
ALTER TABLE operations
  DROP COLUMN IF EXISTS parent_operation_id,
  DROP COLUMN IF EXISTS deadline_at,
  DROP COLUMN IF EXISTS input;
