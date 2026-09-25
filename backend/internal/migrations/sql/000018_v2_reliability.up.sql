ALTER TABLE outbox_events
  ADD COLUMN last_error text,
  ADD COLUMN dead_lettered_at timestamptz,
  ADD COLUMN replayed_at timestamptz,
  ADD COLUMN replayed_by uuid REFERENCES admins(id);

CREATE INDEX ix_outbox_dead_letters ON outbox_events(dead_lettered_at DESC) WHERE status = 'dead_letter';

INSERT INTO permissions(id,key)
VALUES ('20000000-0000-0000-0000-000000000002','outbox.replay')
ON CONFLICT(key) DO NOTHING;

INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE r.key IN ('super_admin','operations') AND p.key='outbox.replay'
ON CONFLICT DO NOTHING;
