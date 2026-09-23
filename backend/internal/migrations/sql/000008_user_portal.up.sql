CREATE UNIQUE INDEX ux_operations_active_instance_action
ON operations(resource_id)
WHERE resource_type = 'instance'
  AND type IN ('start', 'stop', 'restart', 'reinstall')
  AND status IN ('queued', 'running', 'waiting_provider', 'waiting_resource', 'verifying', 'retrying');

ALTER TABLE tickets
  ADD CONSTRAINT tickets_status_valid CHECK (status IN ('open', 'waiting_user', 'waiting_support', 'resolved', 'closed')),
  ADD CONSTRAINT tickets_priority_valid CHECK (priority IN ('low', 'normal', 'high'));
