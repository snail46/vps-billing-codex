INSERT INTO permissions (id, key)
SELECT gen_random_uuid(), permission_key
FROM unnest(ARRAY[
  'health.read', 'products.read', 'products.manage', 'orders.read',
  'subscriptions.read', 'admins.read', 'roles.read', 'settings.read'
]) AS permission_key
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles CROSS JOIN permissions
WHERE roles.key = 'super_admin'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles JOIN permissions ON permissions.key IN (
  'health.read', 'orders.read', 'subscriptions.read', 'instances.read',
  'nodes.read', 'providers.read', 'operations.read', 'audit.read'
)
WHERE roles.key = 'operations'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles JOIN permissions ON permissions.key IN (
  'users.read', 'products.read', 'orders.read', 'payments.read', 'ledger.read', 'audit.read'
)
WHERE roles.key = 'finance'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles JOIN permissions ON permissions.key IN (
  'users.read', 'orders.read', 'subscriptions.read', 'instances.read', 'tickets.read', 'tickets.reply'
)
WHERE roles.key = 'support'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles JOIN permissions ON permissions.key LIKE '%.read'
WHERE roles.key = 'read_only'
ON CONFLICT DO NOTHING;
