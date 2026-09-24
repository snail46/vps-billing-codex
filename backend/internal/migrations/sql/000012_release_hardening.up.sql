CREATE INDEX ix_outbox_pending_dispatch
ON outbox_events(next_attempt_at, created_at)
WHERE status = 'pending';

CREATE INDEX ix_operations_failed_recent
ON operations(created_at DESC)
WHERE status = 'failed';

CREATE INDEX ix_payments_succeeded_paid
ON payments(paid_at DESC)
WHERE status = 'succeeded';

CREATE INDEX ix_audit_events_created
ON audit_events(created_at DESC);

CREATE INDEX ix_tickets_updated
ON tickets(updated_at DESC);

CREATE INDEX ix_notifications_user_unread
ON notifications(user_id, created_at DESC)
WHERE read_at IS NULL AND user_id IS NOT NULL;
