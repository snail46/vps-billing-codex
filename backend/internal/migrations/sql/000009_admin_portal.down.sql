DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key IN (
  'health.read', 'products.read', 'products.manage', 'orders.read',
  'subscriptions.read', 'admins.read', 'roles.read', 'settings.read'
));
DELETE FROM permissions WHERE key IN (
  'health.read', 'products.read', 'products.manage', 'orders.read',
  'subscriptions.read', 'admins.read', 'roles.read', 'settings.read'
);
