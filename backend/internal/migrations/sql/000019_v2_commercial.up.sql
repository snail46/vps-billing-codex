CREATE TABLE promotions (
  id uuid PRIMARY KEY,
  code varchar(64) NOT NULL UNIQUE,
  status varchar(32) NOT NULL CHECK(status IN ('active','disabled')),
  discount_type varchar(16) NOT NULL CHECK(discount_type IN ('fixed','percent')),
  discount_value bigint NOT NULL CHECK(discount_value > 0),
  currency varchar(3),
  starts_at timestamptz,
  ends_at timestamptz,
  max_redemptions integer CHECK(max_redemptions IS NULL OR max_redemptions > 0),
  redemption_count integer NOT NULL DEFAULT 0 CHECK(redemption_count >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK(discount_type <> 'percent' OR discount_value <= 10000),
  CHECK(discount_type <> 'fixed' OR currency IS NOT NULL)
);

CREATE TABLE promotion_redemptions (
  id uuid PRIMARY KEY,
  promotion_id uuid NOT NULL REFERENCES promotions(id),
  order_id uuid NOT NULL UNIQUE REFERENCES orders(id),
  user_id uuid NOT NULL REFERENCES users(id),
  discount_minor bigint NOT NULL CHECK(discount_minor >= 0),
  snapshot jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE orders ADD COLUMN promotion_snapshot jsonb;

CREATE TABLE billing_profiles (
  user_id uuid PRIMARY KEY REFERENCES users(id),
  legal_name varchar(255) NOT NULL DEFAULT '',
  tax_id varchar(128) NOT NULL DEFAULT '',
  country_code varchar(2) NOT NULL DEFAULT '',
  address jsonb NOT NULL DEFAULT '{}'::jsonb,
  updated_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE invoices ADD COLUMN billing_profile_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE refunds (
  id uuid PRIMARY KEY,
  payment_id uuid NOT NULL REFERENCES payments(id),
  amount_minor bigint NOT NULL CHECK(amount_minor > 0),
  currency varchar(3) NOT NULL,
  reason text NOT NULL,
  status varchar(32) NOT NULL CHECK(status IN ('succeeded','failed')),
  idempotency_key varchar(255) NOT NULL UNIQUE,
  ledger_transaction_id uuid NOT NULL REFERENCES ledger_transactions(id),
  created_by uuid REFERENCES admins(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_refunds_payment ON refunds(payment_id,created_at);
ALTER TABLE payments ADD COLUMN refunded_minor bigint NOT NULL DEFAULT 0 CHECK(refunded_minor >= 0 AND refunded_minor <= amount_minor);

INSERT INTO permissions(id,key)
VALUES ('20000000-0000-0000-0000-000000000003','payments.refund')
ON CONFLICT(key) DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE r.key IN ('super_admin','finance') AND p.key='payments.refund'
ON CONFLICT DO NOTHING;
