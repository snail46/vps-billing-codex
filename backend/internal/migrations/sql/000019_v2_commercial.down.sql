DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE key='payments.refund');
DELETE FROM permissions WHERE key='payments.refund';
ALTER TABLE payments DROP COLUMN refunded_minor;
DROP TABLE IF EXISTS refunds;
ALTER TABLE invoices DROP COLUMN billing_profile_snapshot;
DROP TABLE IF EXISTS billing_profiles;
ALTER TABLE orders DROP COLUMN promotion_snapshot;
DROP TABLE IF EXISTS promotion_redemptions;
DROP TABLE IF EXISTS promotions;
