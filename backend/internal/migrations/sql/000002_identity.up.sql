CREATE TABLE user_sessions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash bytea NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ix_user_sessions_active
ON user_sessions(user_id, expires_at)
WHERE revoked_at IS NULL;

CREATE TABLE admin_sessions (
  id uuid PRIMARY KEY,
  admin_id uuid NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
  token_hash bytea NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ix_admin_sessions_active
ON admin_sessions(admin_id, expires_at)
WHERE revoked_at IS NULL;

CREATE TABLE admin_totp_secrets (
  admin_id uuid PRIMARY KEY REFERENCES admins(id) ON DELETE CASCADE,
  ciphertext bytea NOT NULL,
  nonce bytea NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  verified_at timestamptz
);

INSERT INTO roles (id, key, name_key)
VALUES
  (gen_random_uuid(), 'super_admin', 'roles.super_admin'),
  (gen_random_uuid(), 'operations', 'roles.operations'),
  (gen_random_uuid(), 'finance', 'roles.finance'),
  (gen_random_uuid(), 'support', 'roles.support'),
  (gen_random_uuid(), 'read_only', 'roles.read_only')
ON CONFLICT (key) DO NOTHING;

INSERT INTO permissions (id, key)
SELECT gen_random_uuid(), permission_key
FROM unnest(ARRAY[
  'users.read', 'users.update', 'users.suspend',
  'instances.read', 'instances.start', 'instances.stop', 'instances.restart', 'instances.reinstall', 'instances.delete',
  'payments.read', 'payments.refund', 'ledger.read', 'ledger.adjust',
  'nodes.read', 'nodes.update', 'nodes.delete', 'providers.read', 'providers.manage',
  'operations.read', 'operations.retry', 'tickets.read', 'tickets.reply', 'tickets.manage',
  'audit.read', 'admins.manage', 'roles.manage', 'settings.manage'
]) AS permission_key
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles
CROSS JOIN permissions
WHERE roles.key = 'super_admin'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles
JOIN permissions ON permissions.key IN (
  'instances.read', 'instances.start', 'instances.stop', 'instances.restart', 'instances.reinstall', 'instances.delete',
  'nodes.read', 'nodes.update', 'providers.read', 'operations.read', 'operations.retry'
)
WHERE roles.key = 'operations'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles
JOIN permissions ON permissions.key IN ('payments.read', 'payments.refund', 'ledger.read', 'ledger.adjust', 'audit.read')
WHERE roles.key = 'finance'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles
JOIN permissions ON permissions.key IN ('users.read', 'instances.read', 'payments.read', 'tickets.read', 'tickets.reply')
WHERE roles.key = 'support'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles
JOIN permissions ON permissions.key LIKE '%.read'
WHERE roles.key = 'read_only'
ON CONFLICT DO NOTHING;
