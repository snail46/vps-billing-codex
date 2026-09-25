DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE key='usage.read');
DELETE FROM permissions WHERE key='usage.read';
DROP TABLE IF EXISTS usage_charges;
DROP TABLE IF EXISTS usage_billing_periods;
DROP TABLE IF EXISTS usage_samples;
