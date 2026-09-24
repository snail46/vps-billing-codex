DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM roles WHERE key IN ('super_admin', 'operations', 'finance', 'support', 'read_only'));
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key IN (
  'users.read', 'users.update', 'users.suspend',
  'instances.read', 'instances.start', 'instances.stop', 'instances.restart', 'instances.reinstall', 'instances.delete',
  'payments.read', 'payments.refund', 'ledger.read', 'ledger.adjust',
  'nodes.read', 'nodes.update', 'nodes.delete', 'providers.read', 'providers.manage',
  'operations.read', 'operations.retry', 'tickets.read', 'tickets.reply', 'tickets.manage',
  'audit.read', 'admins.manage', 'roles.manage', 'settings.manage'
));
DELETE FROM permissions WHERE key IN (
  'users.read', 'users.update', 'users.suspend',
  'instances.read', 'instances.start', 'instances.stop', 'instances.restart', 'instances.reinstall', 'instances.delete',
  'payments.read', 'payments.refund', 'ledger.read', 'ledger.adjust',
  'nodes.read', 'nodes.update', 'nodes.delete', 'providers.read', 'providers.manage',
  'operations.read', 'operations.retry', 'tickets.read', 'tickets.reply', 'tickets.manage',
  'audit.read', 'admins.manage', 'roles.manage', 'settings.manage'
);
DELETE FROM roles WHERE key IN ('super_admin', 'operations', 'finance', 'support', 'read_only');
DROP TABLE IF EXISTS admin_sessions;
DROP TABLE IF EXISTS admin_totp_secrets;
DROP TABLE IF EXISTS user_sessions;
