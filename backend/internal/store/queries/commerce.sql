-- name: ListActiveProductsAndPlans :many
SELECT products.id AS product_id, products.slug AS product_slug, products.name_i18n AS product_name_i18n,
       products.description_i18n, products.product_type, products.featured,
       plans.id AS plan_id, plans.slug AS plan_slug, plans.name_i18n AS plan_name_i18n,
       plans.cpu_cores, plans.memory_mb, plans.disk_gb, plans.traffic_gb, plans.bandwidth_mbps,
       plans.ipv4_count, plans.ipv6_count, plans.nat_port_count, plans.virtualization, plans.billing_cycle,
       plans.price_minor, plans.currency, plans.stock_mode, plans.stock_quantity, plans.setup_fee_minor,
       plans.traffic_overage_price_minor, node_groups.region,
       (CASE WHEN plans.stock_mode = 'manual' THEN COALESCE(plans.stock_quantity, 0) > 0
            ELSE EXISTS (
              SELECT 1 FROM nodes
              WHERE nodes.node_group_id = plans.node_group_id AND nodes.status = 'online'
                AND nodes.cpu_total - nodes.cpu_allocated - nodes.cpu_reserved >= plans.cpu_cores
                AND nodes.memory_total_mb - nodes.memory_allocated_mb - nodes.memory_reserved_mb >= plans.memory_mb
                AND nodes.disk_total_gb - nodes.disk_allocated_gb - nodes.disk_reserved_gb >= plans.disk_gb
                AND nodes.ipv4_total - nodes.ipv4_allocated - nodes.ipv4_reserved >= plans.ipv4_count
                AND nodes.ipv6_total - nodes.ipv6_allocated - nodes.ipv6_reserved >= plans.ipv6_count
                AND nodes.nat_port_total - nodes.nat_port_allocated - nodes.nat_port_reserved >= plans.nat_port_count
            ) END)::boolean AS available
FROM products JOIN plans ON plans.product_id = products.id
LEFT JOIN node_groups ON node_groups.id = plans.node_group_id
WHERE products.status = 'active' AND plans.status = 'active'
ORDER BY products.featured DESC, products.sort_order, products.slug, plans.price_minor, plans.slug;

-- name: GetPlanForOrder :one
SELECT plans.*, products.slug AS product_slug, products.name_i18n AS product_name_i18n,
       products.description_i18n AS product_description_i18n, products.status AS product_status
FROM plans JOIN products ON products.id = plans.product_id
WHERE plans.id = $1 AND plans.status = 'active' AND products.status = 'active';

-- name: CreateOrder :one
INSERT INTO orders (id, order_no, user_id, status, subtotal_minor, discount_minor, total_minor, currency, idempotency_key, kind, subscription_id)
VALUES ($1, $2, $3, 'pending', $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetOrderByUserIdempotency :one
SELECT * FROM orders WHERE user_id = $1 AND idempotency_key = $2;

-- name: CreateOrderItem :exec
INSERT INTO order_items (id, order_id, product_id, plan_id, quantity, unit_price_minor, total_minor, product_snapshot, plan_snapshot)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: CreateInvoice :one
INSERT INTO invoices (id, invoice_no, user_id, subscription_id, order_id, status, amount_minor, currency, due_at, billing_profile_snapshot)
VALUES ($1, $2, $3, $4, $5, 'open', $6, $7, $8, COALESCE((SELECT to_jsonb(bp)-'user_id'-'updated_at' FROM billing_profiles bp WHERE bp.user_id=$3),'{}'::jsonb))
RETURNING *;

-- name: CreateInvoiceItem :exec
INSERT INTO invoice_items (id, invoice_id, description_i18n, quantity, unit_amount_minor, total_minor)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: CreatePayment :one
INSERT INTO payments (id, payment_no, order_id, gateway, status, amount_minor, currency, idempotency_key)
VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7)
RETURNING *;

-- name: GetPaymentByOrder :one
SELECT * FROM payments WHERE order_id = $1 ORDER BY created_at LIMIT 1;

-- name: ListOrdersByUser :many
SELECT orders.*, payments.id AS payment_id, payments.status AS payment_status, payments.gateway
FROM orders LEFT JOIN payments ON payments.order_id = orders.id
WHERE orders.user_id = $1 ORDER BY orders.created_at DESC;

-- name: ListInvoicesByUser :many
SELECT * FROM invoices WHERE user_id = $1 ORDER BY created_at DESC;

-- name: GetWalletByUserCurrency :one
SELECT * FROM wallets WHERE user_id = $1 AND currency = $2;

-- name: EnsureWallet :one
INSERT INTO wallets (id, user_id, currency) VALUES ($1, $2, $3)
ON CONFLICT (user_id, currency) DO UPDATE SET currency = EXCLUDED.currency
RETURNING *;

-- name: InsertWebhookReceipt :one
INSERT INTO payment_webhook_receipts (id, gateway, external_event_id, payload)
VALUES ($1, $2, $3, $4)
ON CONFLICT (gateway, external_event_id) DO NOTHING
RETURNING id;

-- name: GetWebhookReceipt :one
SELECT * FROM payment_webhook_receipts WHERE gateway = $1 AND external_event_id = $2;

-- name: LockPaymentOrderInvoice :one
SELECT payments.id AS payment_id, payments.status AS payment_status, payments.amount_minor AS payment_amount_minor,
       payments.currency AS payment_currency, payments.gateway_payment_id, payments.order_id, orders.user_id, orders.status AS order_status,
       invoices.id AS invoice_id, invoices.status AS invoice_status, invoices.subscription_id
FROM payments
JOIN orders ON orders.id = payments.order_id
JOIN invoices ON invoices.order_id = orders.id
WHERE payments.id = $1 AND payments.gateway = $2
FOR UPDATE OF payments, orders, invoices;

-- name: MarkPaymentSucceeded :exec
UPDATE payments SET status = 'succeeded', gateway_payment_id = $2, gateway_payload = $3, paid_at = now(), updated_at = now()
WHERE id = $1;

-- name: MarkOrderPaid :exec
UPDATE orders SET status = 'paid', paid_at = now(), updated_at = now() WHERE id = $1;

-- name: MarkInvoicePaid :exec
UPDATE invoices SET status = 'paid', paid_at = now(), updated_at = now() WHERE id = $1;

-- name: CreateLedgerTransaction :exec
INSERT INTO ledger_transactions (id, type, reference_type, reference_id, description)
VALUES ($1, $2, $3, $4, $5);

-- name: CreateLedgerEntry :exec
INSERT INTO ledger_entries (id, transaction_id, account_type, account_id, direction, amount_minor, currency)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: CreateOutboxEvent :exec
INSERT INTO outbox_events (id, event_type, aggregate_type, aggregate_id, payload)
VALUES ($1, $2, $3, $4, $5);

-- name: MarkWebhookProcessed :exec
UPDATE payment_webhook_receipts SET processed_at = now() WHERE id = $1;

-- name: CountLedgerTransactionsByReference :one
SELECT count(*) FROM ledger_transactions WHERE type = $1 AND reference_type = $2 AND reference_id = $3;

-- name: ClaimOutboxEvents :many
SELECT * FROM outbox_events
WHERE status = 'pending' AND next_attempt_at <= now()
ORDER BY created_at
FOR UPDATE SKIP LOCKED
LIMIT $1;

-- name: MarkOutboxPublished :exec
UPDATE outbox_events SET status = 'published', published_at = now() WHERE id = $1;

-- name: MarkOutboxRetry :exec
UPDATE outbox_events
SET attempts = attempts + 1,
    status = CASE WHEN attempts + 1 >= 10 THEN 'dead_letter' ELSE 'pending' END,
    dead_lettered_at = CASE WHEN attempts + 1 >= 10 THEN now() ELSE NULL END,
    last_error = left(sqlc.arg(error_message), 2000),
    next_attempt_at = now() + make_interval(secs => LEAST(300, (1 << LEAST(attempts, 8))))
WHERE id = sqlc.arg(id);
