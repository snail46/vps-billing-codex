ALTER TABLE orders
  ADD COLUMN kind varchar(32) NOT NULL DEFAULT 'purchase',
  ADD COLUMN subscription_id uuid REFERENCES subscriptions(id),
  ADD CONSTRAINT orders_kind_valid CHECK (kind IN ('purchase', 'renewal')),
  ADD CONSTRAINT orders_kind_subscription_consistent CHECK (
    (kind = 'purchase' AND subscription_id IS NULL) OR
    (kind = 'renewal' AND subscription_id IS NOT NULL)
  );

CREATE INDEX ix_subscriptions_lifecycle_due
ON subscriptions(status, next_due_at, grace_until, current_period_end);

ALTER TABLE subscriptions
  ADD CONSTRAINT subscriptions_status_valid CHECK (
    status IN ('pending', 'active', 'past_due', 'suspended', 'cancelled', 'expired', 'terminated')
  );

ALTER TABLE subscriptions
  ADD CONSTRAINT subscriptions_billing_cycle_valid CHECK (
    billing_cycle IN ('monthly', 'quarterly', 'yearly')
  );
