CREATE TABLE usage_samples (
  id uuid PRIMARY KEY,
  provider_id uuid NOT NULL REFERENCES providers(id),
  instance_id uuid NOT NULL REFERENCES instances(id),
  source_event_id varchar(255) NOT NULL,
  period_start timestamptz NOT NULL,
  period_end timestamptz NOT NULL,
  rx_bytes bigint NOT NULL CHECK (rx_bytes >= 0),
  tx_bytes bigint NOT NULL CHECK (tx_bytes >= 0),
  collected_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(provider_id, source_event_id),
  CHECK (period_end > period_start)
);

CREATE INDEX ix_usage_samples_instance_period
ON usage_samples(instance_id, period_start, period_end);

CREATE TABLE usage_billing_periods (
  id uuid PRIMARY KEY,
  subscription_id uuid NOT NULL REFERENCES subscriptions(id),
  instance_id uuid NOT NULL REFERENCES instances(id),
  period_start timestamptz NOT NULL,
  period_end timestamptz NOT NULL,
  included_bytes bigint NOT NULL CHECK (included_bytes >= 0),
  used_bytes bigint NOT NULL DEFAULT 0 CHECK (used_bytes >= 0),
  overage_bytes bigint NOT NULL DEFAULT 0 CHECK (overage_bytes >= 0),
  overage_price_minor_per_gb bigint NOT NULL CHECK (overage_price_minor_per_gb >= 0),
  amount_minor bigint NOT NULL DEFAULT 0 CHECK (amount_minor >= 0),
  currency varchar(3) NOT NULL,
  status varchar(32) NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed','invoiced')),
  invoice_id uuid REFERENCES invoices(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(subscription_id, period_start)
);

CREATE INDEX ix_usage_billing_periods_due
ON usage_billing_periods(period_end, id)
WHERE status = 'open';

CREATE TABLE usage_charges (
  id uuid PRIMARY KEY,
  usage_billing_period_id uuid NOT NULL UNIQUE REFERENCES usage_billing_periods(id),
  invoice_id uuid NOT NULL REFERENCES invoices(id),
  ledger_transaction_id uuid NOT NULL UNIQUE REFERENCES ledger_transactions(id),
  amount_minor bigint NOT NULL CHECK (amount_minor > 0),
  currency varchar(3) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO permissions(id,key) VALUES ('10000000-0000-7000-8000-000000000201','usage.read') ON CONFLICT(key) DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r JOIN permissions p ON p.key='usage.read'
WHERE r.key IN ('super_admin','operations','finance','read_only') ON CONFLICT DO NOTHING;
