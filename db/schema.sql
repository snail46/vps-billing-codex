-- Consolidated V1 reference schema. Production must use versioned migrations.

CREATE TABLE users (
  id uuid PRIMARY KEY,
  email varchar(320) NOT NULL UNIQUE,
  password_hash text NOT NULL,
  status varchar(64) NOT NULL,
  locale varchar(16) NOT NULL DEFAULT 'zh-CN',
  timezone varchar(64) NOT NULL DEFAULT 'UTC',
  email_verified_at timestamptz,
  last_login_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE admins (
  id uuid PRIMARY KEY,
  email varchar(320) NOT NULL UNIQUE,
  password_hash text NOT NULL,
  status varchar(64) NOT NULL,
  display_name varchar(255),
  two_factor_enabled boolean NOT NULL DEFAULT false,
  last_login_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_sessions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash bytea NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE admin_sessions (
  id uuid PRIMARY KEY,
  admin_id uuid NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
  token_hash bytea NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE admin_totp_secrets (
  admin_id uuid PRIMARY KEY REFERENCES admins(id) ON DELETE CASCADE,
  ciphertext bytea NOT NULL,
  nonce bytea NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  verified_at timestamptz
);

CREATE TABLE roles (
  id uuid PRIMARY KEY,
  key varchar(128) NOT NULL UNIQUE,
  name_key varchar(255) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE permissions (
  id uuid PRIMARY KEY,
  key varchar(255) NOT NULL UNIQUE
);

CREATE TABLE admin_roles (
  admin_id uuid NOT NULL REFERENCES admins(id),
  role_id uuid NOT NULL REFERENCES roles(id),
  PRIMARY KEY (admin_id, role_id)
);

CREATE TABLE role_permissions (
  role_id uuid NOT NULL REFERENCES roles(id),
  permission_id uuid NOT NULL REFERENCES permissions(id),
  PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE node_groups (
  id uuid PRIMARY KEY,
  name varchar(255) NOT NULL UNIQUE,
  region varchar(128) NOT NULL,
  status varchar(64) NOT NULL CHECK (status IN ('active', 'disabled')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE products (
  id uuid PRIMARY KEY,
  slug varchar(255) NOT NULL UNIQUE,
  name_i18n jsonb NOT NULL,
  description_i18n jsonb NOT NULL DEFAULT '{}'::jsonb,
  status varchar(64) NOT NULL,
  sort_order integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE plans (
  id uuid PRIMARY KEY,
  product_id uuid NOT NULL REFERENCES products(id),
  node_group_id uuid REFERENCES node_groups(id),
  slug varchar(255) NOT NULL,
  name_i18n jsonb NOT NULL,
  status varchar(64) NOT NULL,
  cpu_cores numeric(10,2) NOT NULL,
  memory_mb integer NOT NULL,
  disk_gb integer NOT NULL,
  traffic_gb bigint,
  bandwidth_mbps integer,
  ipv4_count integer NOT NULL DEFAULT 0,
  ipv6_count integer NOT NULL DEFAULT 0,
  nat_port_count integer NOT NULL DEFAULT 0,
  virtualization varchar(64) NOT NULL,
  billing_cycle varchar(64) NOT NULL,
  price_minor bigint NOT NULL CHECK (price_minor >= 0),
  currency varchar(3) NOT NULL,
  stock_mode varchar(64) NOT NULL DEFAULT 'automatic',
  default_image_id varchar(255) NOT NULL DEFAULT 'ubuntu-24.04',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(product_id, slug)
);

CREATE TABLE providers (
  id uuid PRIMARY KEY,
  name varchar(255) NOT NULL UNIQUE,
  provider_type varchar(64) NOT NULL,
  endpoint text,
  credential_ref varchar(255),
  status varchar(64) NOT NULL CHECK (status IN ('active', 'degraded', 'disabled', 'unavailable')),
  version varchar(128),
  config jsonb NOT NULL DEFAULT '{}'::jsonb,
  capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
  last_health_check_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE nodes (
  id uuid PRIMARY KEY,
  provider_id uuid NOT NULL REFERENCES providers(id),
  node_group_id uuid REFERENCES node_groups(id),
  provider_node_id varchar(255),
  name varchar(255) NOT NULL UNIQUE,
  region varchar(128) NOT NULL,
  status varchar(64) NOT NULL CHECK (status IN ('online', 'degraded', 'draining', 'maintenance', 'offline')),
  cpu_total numeric(10,2) NOT NULL DEFAULT 0,
  memory_total_mb bigint NOT NULL DEFAULT 0,
  disk_total_gb bigint NOT NULL DEFAULT 0,
  cpu_allocated numeric(10,2) NOT NULL DEFAULT 0,
  memory_allocated_mb bigint NOT NULL DEFAULT 0,
  disk_allocated_gb bigint NOT NULL DEFAULT 0,
  cpu_reserved numeric(10,2) NOT NULL DEFAULT 0,
  memory_reserved_mb bigint NOT NULL DEFAULT 0,
  disk_reserved_gb bigint NOT NULL DEFAULT 0,
  ipv4_total integer NOT NULL DEFAULT 0,
  ipv4_allocated integer NOT NULL DEFAULT 0,
  ipv4_reserved integer NOT NULL DEFAULT 0,
  ipv6_total integer NOT NULL DEFAULT 0,
  ipv6_allocated integer NOT NULL DEFAULT 0,
  ipv6_reserved integer NOT NULL DEFAULT 0,
  nat_port_total integer NOT NULL DEFAULT 0,
  nat_port_allocated integer NOT NULL DEFAULT 0,
  nat_port_reserved integer NOT NULL DEFAULT 0,
  weight integer NOT NULL DEFAULT 100,
  capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
  last_seen_at timestamptz,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT nodes_capacity_nonnegative CHECK (
    cpu_total >= 0 AND memory_total_mb >= 0 AND disk_total_gb >= 0 AND
    cpu_allocated >= 0 AND memory_allocated_mb >= 0 AND disk_allocated_gb >= 0 AND
    cpu_reserved >= 0 AND memory_reserved_mb >= 0 AND disk_reserved_gb >= 0 AND
    ipv4_total >= 0 AND ipv4_allocated >= 0 AND ipv4_reserved >= 0 AND
    ipv6_total >= 0 AND ipv6_allocated >= 0 AND ipv6_reserved >= 0 AND
    nat_port_total >= 0 AND nat_port_allocated >= 0 AND nat_port_reserved >= 0
  ),
  CONSTRAINT nodes_capacity_not_oversubscribed CHECK (
    cpu_allocated + cpu_reserved <= cpu_total AND
    memory_allocated_mb + memory_reserved_mb <= memory_total_mb AND
    disk_allocated_gb + disk_reserved_gb <= disk_total_gb AND
    ipv4_allocated + ipv4_reserved <= ipv4_total AND
    ipv6_allocated + ipv6_reserved <= ipv6_total AND
    nat_port_allocated + nat_port_reserved <= nat_port_total
  )
);

CREATE UNIQUE INDEX ux_nodes_provider_node
ON nodes(provider_id, provider_node_id)
WHERE provider_node_id IS NOT NULL;

CREATE TABLE orders (
  id uuid PRIMARY KEY,
  order_no varchar(64) NOT NULL UNIQUE,
  user_id uuid NOT NULL REFERENCES users(id),
  status varchar(64) NOT NULL,
  subtotal_minor bigint NOT NULL,
  discount_minor bigint NOT NULL DEFAULT 0,
  total_minor bigint NOT NULL,
  currency varchar(3) NOT NULL,
  kind varchar(32) NOT NULL DEFAULT 'purchase' CHECK (kind IN ('purchase', 'renewal')),
  subscription_id uuid,
  idempotency_key varchar(255),
  paid_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE order_items (
  id uuid PRIMARY KEY,
  order_id uuid NOT NULL REFERENCES orders(id),
  product_id uuid NOT NULL REFERENCES products(id),
  plan_id uuid NOT NULL REFERENCES plans(id),
  quantity integer NOT NULL CHECK (quantity > 0),
  unit_price_minor bigint NOT NULL,
  total_minor bigint NOT NULL,
  product_snapshot jsonb NOT NULL,
  plan_snapshot jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE payments (
  id uuid PRIMARY KEY,
  payment_no varchar(64) NOT NULL UNIQUE,
  order_id uuid NOT NULL REFERENCES orders(id),
  gateway varchar(64) NOT NULL,
  gateway_payment_id varchar(255),
  status varchar(64) NOT NULL,
  amount_minor bigint NOT NULL,
  currency varchar(3) NOT NULL,
  idempotency_key varchar(255) NOT NULL UNIQUE,
  gateway_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  paid_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ux_payments_gateway_external
ON payments(gateway, gateway_payment_id)
WHERE gateway_payment_id IS NOT NULL;

CREATE UNIQUE INDEX ux_orders_user_idempotency
ON orders(user_id, idempotency_key)
WHERE idempotency_key IS NOT NULL;

CREATE TABLE payment_webhook_receipts (
  id uuid PRIMARY KEY,
  gateway varchar(64) NOT NULL,
  external_event_id varchar(255) NOT NULL,
  payload jsonb NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now(),
  processed_at timestamptz,
  UNIQUE(gateway, external_event_id)
);

CREATE TABLE wallets (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  currency varchar(3) NOT NULL,
  available_balance_minor bigint NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id, currency)
);

CREATE TABLE ledger_transactions (
  id uuid PRIMARY KEY,
  type varchar(64) NOT NULL,
  reference_type varchar(64),
  reference_id uuid,
  description text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ledger_entries (
  id uuid PRIMARY KEY,
  transaction_id uuid NOT NULL REFERENCES ledger_transactions(id),
  account_type varchar(64) NOT NULL,
  account_id uuid NOT NULL,
  direction varchar(16) NOT NULL CHECK (direction IN ('debit','credit')),
  amount_minor bigint NOT NULL CHECK (amount_minor >= 0),
  currency varchar(3) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ux_ledger_transaction_reference
ON ledger_transactions(type, reference_type, reference_id)
WHERE reference_type IS NOT NULL AND reference_id IS NOT NULL;

CREATE FUNCTION reject_ledger_mutation() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'ledger history is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER ledger_transactions_immutable
BEFORE UPDATE OR DELETE ON ledger_transactions
FOR EACH ROW EXECUTE FUNCTION reject_ledger_mutation();

CREATE TRIGGER ledger_entries_immutable
BEFORE UPDATE OR DELETE ON ledger_entries
FOR EACH ROW EXECUTE FUNCTION reject_ledger_mutation();

CREATE TABLE subscriptions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  plan_id uuid NOT NULL REFERENCES plans(id),
  status varchar(64) NOT NULL CHECK (status IN ('pending', 'active', 'past_due', 'suspended', 'cancelled', 'expired', 'terminated')),
  billing_cycle varchar(64) NOT NULL CHECK (billing_cycle IN ('monthly', 'quarterly', 'yearly')),
  price_minor bigint NOT NULL,
  currency varchar(3) NOT NULL,
  started_at timestamptz,
  current_period_start timestamptz,
  current_period_end timestamptz,
  next_due_at timestamptz,
  grace_until timestamptz,
  cancel_at_period_end boolean NOT NULL DEFAULT false,
  ended_at timestamptz,
  version bigint NOT NULL DEFAULT 1,
  source_order_id uuid REFERENCES orders(id),
  source_item_index integer,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT subscriptions_source_consistent CHECK (
    (source_order_id IS NULL AND source_item_index IS NULL) OR
    (source_order_id IS NOT NULL AND source_item_index > 0)
  )
);

CREATE UNIQUE INDEX ux_subscriptions_source_order_item
ON subscriptions(source_order_id, source_item_index)
WHERE source_order_id IS NOT NULL;

CREATE INDEX ix_subscriptions_lifecycle_due
ON subscriptions(status, next_due_at, grace_until, current_period_end);

ALTER TABLE orders
  ADD CONSTRAINT fk_orders_subscription FOREIGN KEY (subscription_id) REFERENCES subscriptions(id),
  ADD CONSTRAINT orders_kind_subscription_consistent CHECK (
    (kind = 'purchase' AND subscription_id IS NULL) OR
    (kind = 'renewal' AND subscription_id IS NOT NULL)
  );

CREATE TABLE invoices (
  id uuid PRIMARY KEY,
  invoice_no varchar(64) NOT NULL UNIQUE,
  user_id uuid NOT NULL REFERENCES users(id),
  subscription_id uuid REFERENCES subscriptions(id),
  order_id uuid REFERENCES orders(id),
  status varchar(64) NOT NULL,
  amount_minor bigint NOT NULL,
  currency varchar(3) NOT NULL,
  due_at timestamptz,
  paid_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ux_invoices_order ON invoices(order_id) WHERE order_id IS NOT NULL;

CREATE TABLE invoice_items (
  id uuid PRIMARY KEY,
  invoice_id uuid NOT NULL REFERENCES invoices(id),
  description_i18n jsonb NOT NULL,
  quantity integer NOT NULL CHECK (quantity > 0),
  unit_amount_minor bigint NOT NULL,
  total_minor bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE instances (
  id uuid PRIMARY KEY,
  subscription_id uuid NOT NULL REFERENCES subscriptions(id),
  node_id uuid REFERENCES nodes(id),
  provider_id uuid REFERENCES providers(id),
  provider_instance_id varchar(255),
  name varchar(255) NOT NULL,
  desired_state varchar(64) NOT NULL,
  observed_state varchar(64) NOT NULL,
  cpu_cores numeric(10,2) NOT NULL,
  memory_mb integer NOT NULL,
  disk_gb integer NOT NULL,
  traffic_limit_gb bigint,
  bandwidth_mbps integer,
  image_id varchar(255),
  primary_ipv4 inet,
  primary_ipv6 inet,
  last_synced_at timestamptz,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE UNIQUE INDEX ux_instance_provider_id
ON instances(provider_id, provider_instance_id)
WHERE provider_instance_id IS NOT NULL;

CREATE UNIQUE INDEX ux_instances_subscription ON instances(subscription_id);

ALTER TABLE instances
  ADD CONSTRAINT instances_desired_state_valid CHECK (desired_state IN ('running', 'stopped', 'suspended', 'deleted')),
  ADD CONSTRAINT instances_observed_state_valid CHECK (observed_state IN ('pending', 'provisioning', 'running', 'stopping', 'stopped', 'restarting', 'reinstalling', 'suspending', 'suspended', 'deleting', 'deleted', 'error', 'unknown'));

CREATE TABLE instance_networks (
  id uuid PRIMARY KEY,
  instance_id uuid NOT NULL REFERENCES instances(id),
  type varchar(32) NOT NULL,
  address inet,
  gateway inet,
  prefix integer,
  provider_network_id varchar(255),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE port_forwards (
  id uuid PRIMARY KEY,
  instance_id uuid NOT NULL REFERENCES instances(id),
  protocol varchar(16) NOT NULL,
  public_ip inet NOT NULL,
  public_port integer NOT NULL CHECK (public_port BETWEEN 1 AND 65535),
  guest_port integer NOT NULL CHECK (guest_port BETWEEN 1 AND 65535),
  description varchar(255),
  status varchar(64) NOT NULL,
  provider_mapping_id varchar(255),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ux_port_forward
ON port_forwards(public_ip, protocol, public_port);

CREATE TABLE traffic_usage (
  id uuid PRIMARY KEY,
  instance_id uuid NOT NULL REFERENCES instances(id),
  period_start timestamptz NOT NULL,
  period_end timestamptz NOT NULL,
  rx_bytes bigint NOT NULL DEFAULT 0,
  tx_bytes bigint NOT NULL DEFAULT 0,
  source varchar(64) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(instance_id, period_start, source)
);

CREATE TABLE operations (
  id uuid PRIMARY KEY,
  type varchar(128) NOT NULL,
  resource_type varchar(64) NOT NULL,
  resource_id uuid NOT NULL,
  status varchar(64) NOT NULL,
  phase varchar(128),
  progress integer NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
  message_key varchar(255),
  provider_id uuid REFERENCES providers(id),
  provider_operation_id varchar(255),
  idempotency_key varchar(255) NOT NULL UNIQUE,
  retryable boolean NOT NULL DEFAULT false,
  retry_count integer NOT NULL DEFAULT 0,
  max_retries integer NOT NULL DEFAULT 0,
  error_code varchar(128),
  error_message text,
  trace_id varchar(255) NOT NULL,
  user_id uuid REFERENCES users(id),
  actor_admin_id uuid REFERENCES admins(id),
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  heartbeat_at timestamptz,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT operations_status_valid CHECK (status IN ('queued', 'running', 'waiting_provider', 'waiting_resource', 'verifying', 'retrying', 'succeeded', 'failed', 'cancelled'))
);

CREATE INDEX ix_operations_dispatch
ON operations(status, next_attempt_at, created_at);

CREATE INDEX ix_operations_user
ON operations(user_id, created_at DESC)
WHERE user_id IS NOT NULL;

CREATE TABLE operation_steps (
  id uuid PRIMARY KEY,
  operation_id uuid NOT NULL REFERENCES operations(id),
  step_key varchar(128) NOT NULL,
  step_order integer NOT NULL,
  status varchar(64) NOT NULL,
  progress integer NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
  attempt integer NOT NULL DEFAULT 0,
  error_code varchar(128),
  error_message text,
  output jsonb NOT NULL DEFAULT '{}'::jsonb,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(operation_id, step_key),
  CONSTRAINT operation_steps_status_valid CHECK (status IN ('pending', 'running', 'waiting', 'succeeded', 'failed', 'skipped'))
);

CREATE TABLE resource_reservations (
  id uuid PRIMARY KEY,
  node_id uuid NOT NULL REFERENCES nodes(id),
  operation_id uuid NOT NULL UNIQUE REFERENCES operations(id),
  cpu_cores numeric(10,2) NOT NULL DEFAULT 0,
  memory_mb bigint NOT NULL DEFAULT 0,
  disk_gb bigint NOT NULL DEFAULT 0,
  ipv4_count integer NOT NULL DEFAULT 0,
  ipv6_count integer NOT NULL DEFAULT 0,
  nat_port_count integer NOT NULL DEFAULT 0,
  status varchar(64) NOT NULL CHECK (status IN ('reserved', 'committed', 'released', 'expired')),
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT resource_reservations_amounts_nonnegative CHECK (
    cpu_cores >= 0 AND memory_mb >= 0 AND disk_gb >= 0 AND
    ipv4_count >= 0 AND ipv6_count >= 0 AND nat_port_count >= 0
  )
);

CREATE INDEX ix_resource_reservations_expiry
ON resource_reservations(status, expires_at);

CREATE TABLE outbox_events (
  id uuid PRIMARY KEY,
  event_type varchar(255) NOT NULL,
  aggregate_type varchar(64) NOT NULL,
  aggregate_id uuid NOT NULL,
  payload jsonb NOT NULL,
  status varchar(64) NOT NULL DEFAULT 'pending',
  attempts integer NOT NULL DEFAULT 0,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  published_at timestamptz
);

CREATE TABLE notifications (
  id uuid PRIMARY KEY,
  user_id uuid REFERENCES users(id),
  admin_id uuid REFERENCES admins(id),
  type varchar(128) NOT NULL,
  title_key varchar(255) NOT NULL,
  message_key varchar(255) NOT NULL,
  parameters jsonb NOT NULL DEFAULT '{}'::jsonb,
  severity varchar(32) NOT NULL,
  read_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((user_id IS NOT NULL)::int + (admin_id IS NOT NULL)::int = 1)
);

CREATE TABLE tickets (
  id uuid PRIMARY KEY,
  ticket_no varchar(64) NOT NULL UNIQUE,
  user_id uuid NOT NULL REFERENCES users(id),
  subject varchar(255) NOT NULL,
  status varchar(64) NOT NULL,
  priority varchar(32) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  closed_at timestamptz
);

CREATE TABLE ticket_messages (
  id uuid PRIMARY KEY,
  ticket_id uuid NOT NULL REFERENCES tickets(id),
  sender_type varchar(32) NOT NULL,
  sender_id uuid,
  message text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_events (
  id uuid PRIMARY KEY,
  actor_type varchar(32) NOT NULL,
  actor_id uuid,
  action varchar(255) NOT NULL,
  resource_type varchar(64) NOT NULL,
  resource_id uuid,
  before_data jsonb,
  after_data jsonb,
  ip_address inet,
  user_agent text,
  request_id varchar(255),
  trace_id varchar(255),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE system_settings (
  key varchar(255) PRIMARY KEY,
  value jsonb NOT NULL,
  is_secret boolean NOT NULL DEFAULT false,
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by uuid
);
