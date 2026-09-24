CREATE INDEX ix_operations_stuck_recovery
ON operations(heartbeat_at, created_at)
WHERE status IN ('running', 'waiting_provider', 'waiting_resource', 'verifying');

CREATE INDEX ix_agent_connections_heartbeat_expiry
ON agent_connections(last_heartbeat_at)
WHERE status = 'connected';

CREATE INDEX ix_instances_reconciliation
ON instances(last_synced_at, id)
WHERE deleted_at IS NULL AND provider_id IS NOT NULL AND provider_instance_id IS NOT NULL;
