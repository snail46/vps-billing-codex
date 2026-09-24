-- name: GetPaidPurchaseForProvision :one
SELECT orders.id AS order_id, orders.user_id, orders.status AS order_status,
       order_items.quantity, plans.*
FROM orders
JOIN order_items ON order_items.order_id = orders.id
JOIN plans ON plans.id = order_items.plan_id
WHERE orders.id = $1 AND orders.kind = 'purchase' AND orders.status IN ('paid', 'fulfilling', 'fulfilled')
FOR UPDATE OF orders;

-- name: CreatePurchaseSubscription :one
INSERT INTO subscriptions (
  id, user_id, plan_id, status, billing_cycle, price_minor, currency,
  source_order_id, source_item_index
)
VALUES ($1, $2, $3, 'pending', $4, $5, $6, $7, $8)
ON CONFLICT (source_order_id, source_item_index) WHERE source_order_id IS NOT NULL
DO UPDATE SET source_order_id = EXCLUDED.source_order_id
RETURNING *;

-- name: CreatePendingInstance :one
INSERT INTO instances (
  id, subscription_id, name, desired_state, observed_state, cpu_cores, memory_mb,
  disk_gb, traffic_limit_gb, bandwidth_mbps, image_id
)
VALUES ($1, $2, $3, 'running', 'pending', $4, $5, $6, $7, $8, $9)
ON CONFLICT (subscription_id) DO UPDATE SET subscription_id = EXCLUDED.subscription_id
RETURNING *;

-- name: MarkOrderFulfilling :exec
UPDATE orders SET status = 'fulfilling', updated_at = now()
WHERE id = $1 AND status = 'paid';

-- name: GetProvisionContext :one
SELECT operations.id AS operation_id, operations.retry_count,
       instances.id AS instance_id, instances.name AS instance_name,
       instances.provider_instance_id, instances.observed_state,
       subscriptions.id AS subscription_id, subscriptions.user_id,
       subscriptions.source_order_id, subscriptions.status AS subscription_status,
       plans.node_group_id, plans.cpu_cores, plans.memory_mb, plans.disk_gb,
       plans.traffic_gb, plans.bandwidth_mbps, plans.ipv4_count, plans.ipv6_count,
       plans.nat_port_count, plans.virtualization, plans.default_image_id
FROM operations
JOIN instances ON instances.id = operations.resource_id AND operations.resource_type = 'instance'
JOIN subscriptions ON subscriptions.id = instances.subscription_id
JOIN plans ON plans.id = subscriptions.plan_id
WHERE operations.id = $1;

-- name: GetProvisionPlacement :one
SELECT resource_reservations.id AS reservation_id, nodes.id AS node_id,
       nodes.provider_id, COALESCE(nodes.provider_node_id, nodes.id::text) AS provider_node_id
FROM resource_reservations
JOIN nodes ON nodes.id = resource_reservations.node_id
WHERE resource_reservations.operation_id = $1;

-- name: SetInstancePlacement :exec
UPDATE instances
SET node_id = $2, provider_id = $3, observed_state = 'provisioning', version = version + 1, updated_at = now()
WHERE id = $1 AND observed_state IN ('pending', 'provisioning', 'unknown', 'error');

-- name: SetInstanceProviderResult :exec
UPDATE instances
SET provider_instance_id = $2, observed_state = $3, primary_ipv4 = $4, primary_ipv6 = $5,
    last_synced_at = now(), version = version + 1, updated_at = now()
WHERE id = $1;

-- name: SetInstanceProvisionError :exec
UPDATE instances
SET observed_state = 'error', version = version + 1, updated_at = now()
WHERE id = $1 AND observed_state <> 'running';

-- name: ActivateProvisionedSubscription :one
UPDATE subscriptions
SET status = 'active', started_at = COALESCE(started_at, $2),
    current_period_start = COALESCE(current_period_start, $2),
    current_period_end = COALESCE(current_period_end, $2 + CASE billing_cycle
      WHEN 'monthly' THEN interval '1 month'
      WHEN 'quarterly' THEN interval '3 months'
      WHEN 'yearly' THEN interval '1 year'
    END),
    next_due_at = COALESCE(next_due_at, $2 + CASE billing_cycle
      WHEN 'monthly' THEN interval '1 month'
      WHEN 'quarterly' THEN interval '3 months'
      WHEN 'yearly' THEN interval '1 year'
    END),
    version = version + CASE WHEN status = 'pending' THEN 1 ELSE 0 END,
    updated_at = now()
WHERE id = $1 AND status IN ('pending', 'active')
RETURNING *;

-- name: MarkPurchaseOrderFulfilledIfReady :exec
UPDATE orders SET status = 'fulfilled', updated_at = now()
WHERE orders.id = $1 AND orders.status IN ('paid', 'fulfilling', 'fulfilled')
  AND NOT EXISTS (
    SELECT 1 FROM subscriptions
    WHERE source_order_id = $1 AND status <> 'active'
  );

-- name: CreateProvisionNotification :exec
INSERT INTO notifications (id, user_id, type, title_key, message_key, parameters, severity)
VALUES ($1, $2, 'instance_ready', 'notifications.instanceReady.title', 'notifications.instanceReady.message', $3, 'success')
ON CONFLICT (id) DO NOTHING;
