DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE key='outbox.replay');
DELETE FROM permissions WHERE key='outbox.replay';
DROP INDEX IF EXISTS ix_outbox_dead_letters;
ALTER TABLE outbox_events DROP COLUMN replayed_by, DROP COLUMN replayed_at, DROP COLUMN dead_lettered_at, DROP COLUMN last_error;
