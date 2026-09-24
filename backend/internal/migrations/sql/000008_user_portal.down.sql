ALTER TABLE tickets
  DROP CONSTRAINT IF EXISTS tickets_priority_valid,
  DROP CONSTRAINT IF EXISTS tickets_status_valid;

DROP INDEX IF EXISTS ux_operations_active_instance_action;
