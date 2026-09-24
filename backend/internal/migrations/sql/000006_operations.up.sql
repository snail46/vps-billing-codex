ALTER TABLE operations
  ADD COLUMN user_id uuid REFERENCES users(id),
  ADD COLUMN actor_admin_id uuid REFERENCES admins(id),
  ADD COLUMN next_attempt_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN heartbeat_at timestamptz,
  ADD CONSTRAINT operations_status_valid CHECK (
    status IN ('queued', 'running', 'waiting_provider', 'waiting_resource', 'verifying', 'retrying', 'succeeded', 'failed', 'cancelled')
  );

ALTER TABLE operation_steps
  ADD COLUMN output jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD CONSTRAINT operation_steps_status_valid CHECK (
    status IN ('pending', 'running', 'waiting', 'succeeded', 'failed', 'skipped')
  );

CREATE INDEX ix_operations_dispatch
ON operations(status, next_attempt_at, created_at);

CREATE INDEX ix_operations_user
ON operations(user_id, created_at DESC)
WHERE user_id IS NOT NULL;
