ALTER TABLE operations
  ADD COLUMN input jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN deadline_at timestamptz,
  ADD COLUMN parent_operation_id uuid REFERENCES operations(id);

CREATE INDEX ix_operations_deadline
ON operations(deadline_at)
WHERE deadline_at IS NOT NULL AND status IN ('queued','running','waiting_provider','waiting_resource','verifying','retrying');

CREATE TABLE operation_attempts (
  id uuid PRIMARY KEY,
  operation_id uuid NOT NULL REFERENCES operations(id),
  attempt integer NOT NULL,
  status varchar(32) NOT NULL CHECK (status IN ('running','retrying','succeeded','failed','cancelled','expired')),
  error_code varchar(128),
  error_message text,
  worker_id varchar(255),
  started_at timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz,
  UNIQUE(operation_id, attempt)
);

CREATE INDEX ix_operation_attempts_operation ON operation_attempts(operation_id, attempt DESC);

DROP INDEX ux_port_forward;
ALTER TABLE port_forwards
  ADD COLUMN operation_id uuid REFERENCES operations(id),
  ADD COLUMN error_code varchar(128),
  ADD CONSTRAINT port_forwards_status_valid CHECK (status IN ('pending','active','deleting','failed','deleted'));

CREATE UNIQUE INDEX ux_port_forward_active
ON port_forwards(public_ip, protocol, public_port)
WHERE status <> 'deleted';

CREATE UNIQUE INDEX ux_port_forward_provider_mapping
ON port_forwards(instance_id, provider_mapping_id)
WHERE provider_mapping_id IS NOT NULL AND status <> 'deleted';

CREATE INDEX ix_port_forwards_instance_active
ON port_forwards(instance_id, created_at)
WHERE status <> 'deleted';

CREATE UNIQUE INDEX ux_port_forwards_operation
ON port_forwards(operation_id)
WHERE operation_id IS NOT NULL;

CREATE UNIQUE INDEX ux_operations_active_port_forward_add
ON operations(resource_id)
WHERE resource_type = 'instance' AND type = 'port_forward_add'
  AND status IN ('queued','running','waiting_provider','waiting_resource','verifying','retrying');
