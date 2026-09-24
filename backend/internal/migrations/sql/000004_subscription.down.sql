ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS subscriptions_billing_cycle_valid;
ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS subscriptions_status_valid;
DROP INDEX IF EXISTS ix_subscriptions_lifecycle_due;
ALTER TABLE orders
  DROP CONSTRAINT IF EXISTS orders_kind_subscription_consistent,
  DROP CONSTRAINT IF EXISTS orders_kind_valid,
  DROP COLUMN IF EXISTS subscription_id,
  DROP COLUMN IF EXISTS kind;
