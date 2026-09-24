-- name: ListUserInstances :many
SELECT i.id, i.name, i.desired_state, i.observed_state, i.cpu_cores, i.memory_mb,
       i.disk_gb, i.traffic_limit_gb, i.bandwidth_mbps, i.image_id,
       COALESCE(host(i.primary_ipv4), '')::text AS primary_ipv4,
       COALESCE(host(i.primary_ipv6), '')::text AS primary_ipv6,
       i.last_synced_at, i.created_at, i.updated_at,
       s.id AS subscription_id, s.status AS subscription_status, s.current_period_end,
       p.slug AS plan_slug, p.name_i18n AS plan_name_i18n
FROM instances i
JOIN subscriptions s ON s.id = i.subscription_id
JOIN plans p ON p.id = s.plan_id
WHERE s.user_id = $1 AND i.deleted_at IS NULL
ORDER BY i.created_at DESC;

-- name: GetUserInstance :one
SELECT i.id, i.name, i.desired_state, i.observed_state, i.cpu_cores, i.memory_mb,
       i.disk_gb, i.traffic_limit_gb, i.bandwidth_mbps, i.image_id,
       COALESCE(host(i.primary_ipv4), '')::text AS primary_ipv4,
       COALESCE(host(i.primary_ipv6), '')::text AS primary_ipv6,
       i.last_synced_at, i.created_at, i.updated_at,
       s.id AS subscription_id, s.status AS subscription_status, s.current_period_end,
       p.slug AS plan_slug, p.name_i18n AS plan_name_i18n
FROM instances i
JOIN subscriptions s ON s.id = i.subscription_id
JOIN plans p ON p.id = s.plan_id
WHERE i.id = $1 AND s.user_id = $2 AND i.deleted_at IS NULL;

-- name: ListUserInstanceNetworks :many
SELECT n.id, n.type, COALESCE(host(n.address), '')::text AS address,
       COALESCE(host(n.gateway), '')::text AS gateway, n.prefix, n.created_at
FROM instance_networks n
JOIN instances i ON i.id = n.instance_id
JOIN subscriptions s ON s.id = i.subscription_id
WHERE n.instance_id = $1 AND s.user_id = $2
ORDER BY n.type, n.created_at;

-- name: ListUserInstanceTraffic :many
SELECT t.period_start, t.period_end, t.rx_bytes, t.tx_bytes, t.source
FROM traffic_usage t
JOIN instances i ON i.id = t.instance_id
JOIN subscriptions s ON s.id = i.subscription_id
WHERE t.instance_id = $1 AND s.user_id = $2
ORDER BY t.period_start DESC
LIMIT 90;

-- name: GetUserInstanceActionContext :one
SELECT i.id, i.provider_id, i.provider_instance_id, i.node_id, i.image_id,
       i.desired_state, i.observed_state, COALESCE(n.provider_node_id, n.id::text) AS provider_node_id
FROM instances i
JOIN subscriptions s ON s.id = i.subscription_id
JOIN nodes n ON n.id = i.node_id
WHERE i.id = $1 AND s.user_id = $2 AND i.deleted_at IS NULL;

-- name: GetInstanceActionContextByOperation :one
SELECT i.id, i.provider_id, i.provider_instance_id, i.node_id, i.image_id,
       i.desired_state, i.observed_state, COALESCE(n.provider_node_id, n.id::text) AS provider_node_id,
       o.type AS operation_type
FROM operations o
JOIN instances i ON i.id = o.resource_id AND o.resource_type = 'instance'
JOIN nodes n ON n.id = i.node_id
WHERE o.id = $1;

-- name: SetInstanceActionState :exec
UPDATE instances
SET desired_state = $2, observed_state = $3, version = version + 1, updated_at = now()
WHERE id = $1;

-- name: SetInstanceObservedState :exec
UPDATE instances
SET observed_state = $2, last_synced_at = now(), version = version + 1, updated_at = now()
WHERE id = $1;

-- name: ListUserNotifications :many
SELECT id, type, title_key, message_key, parameters, severity, read_at, created_at
FROM notifications
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT 100;

-- name: MarkUserNotificationRead :execrows
UPDATE notifications SET read_at = COALESCE(read_at, now())
WHERE id = $1 AND user_id = $2;

-- name: ListUserTickets :many
SELECT id, ticket_no, subject, status, priority, created_at, updated_at, closed_at
FROM tickets
WHERE user_id = $1
ORDER BY updated_at DESC;

-- name: GetUserTicket :one
SELECT id, ticket_no, subject, status, priority, created_at, updated_at, closed_at
FROM tickets
WHERE id = $1 AND user_id = $2;

-- name: ListUserTicketMessages :many
SELECT m.id, m.sender_type, m.message, m.created_at
FROM ticket_messages m
JOIN tickets t ON t.id = m.ticket_id
WHERE m.ticket_id = $1 AND t.user_id = $2
ORDER BY m.created_at, m.id;

-- name: CreateUserTicket :one
INSERT INTO tickets (id, ticket_no, user_id, subject, status, priority)
VALUES ($1, $2, $3, $4, 'open', $5)
RETURNING *;

-- name: CreateUserTicketMessage :one
INSERT INTO ticket_messages (id, ticket_id, sender_type, sender_id, message)
VALUES ($1, $2, 'user', $3, $4)
RETURNING *;

-- name: TouchUserTicket :exec
UPDATE tickets SET updated_at = now()
WHERE id = $1 AND user_id = $2 AND status <> 'closed';
