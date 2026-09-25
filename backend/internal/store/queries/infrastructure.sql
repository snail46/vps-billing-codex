-- name: CreateInfrastructureProvider :one
INSERT INTO providers (id, name, provider_type, endpoint, credential_ref, status, config, capabilities)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: CreateNodeGroup :one
INSERT INTO node_groups (id, name, region, status)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: CreateNode :one
INSERT INTO nodes (
  id, provider_id, node_group_id, provider_node_id, name, region, status,
  cpu_total, memory_total_mb, disk_total_gb, ipv4_total, ipv6_total, nat_port_total,
  weight, capabilities, last_seen_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING *;

-- name: ListInfrastructureProviders :many
SELECT * FROM providers ORDER BY name, id;

-- name: GetInfrastructureProviderByID :one
SELECT * FROM providers WHERE id = $1 AND status IN ('active', 'degraded');

-- name: ListNodeGroups :many
SELECT * FROM node_groups ORDER BY region, name, id;

-- name: ListNodes :many
SELECT * FROM nodes ORDER BY region, name, id;

-- name: SelectCandidateNodesForUpdate :many
SELECT nodes.*
FROM nodes
JOIN providers ON providers.id = nodes.provider_id
JOIN node_groups ON node_groups.id = nodes.node_group_id
WHERE nodes.node_group_id = sqlc.arg(node_group_id)
  AND nodes.status = 'online'
  AND providers.status = 'active'
  AND node_groups.status = 'active'
  AND nodes.capabilities @> sqlc.arg(required_capabilities)::jsonb
  AND nodes.cpu_total - nodes.cpu_allocated - nodes.cpu_reserved >= sqlc.arg(cpu_cores)
  AND nodes.memory_total_mb - nodes.memory_allocated_mb - nodes.memory_reserved_mb >= sqlc.arg(memory_mb)
  AND nodes.disk_total_gb - nodes.disk_allocated_gb - nodes.disk_reserved_gb >= sqlc.arg(disk_gb)
  AND nodes.ipv4_total - nodes.ipv4_allocated - nodes.ipv4_reserved >= sqlc.arg(ipv4_count)
  AND nodes.ipv6_total - nodes.ipv6_allocated - nodes.ipv6_reserved >= sqlc.arg(ipv6_count)
  AND nodes.nat_port_total - nodes.nat_port_allocated - nodes.nat_port_reserved >= sqlc.arg(nat_port_count)
ORDER BY CASE WHEN node_groups.placement_policy = 'pack' THEN -1 ELSE 1 END * GREATEST(
    COALESCE((nodes.cpu_allocated + nodes.cpu_reserved + sqlc.arg(cpu_cores)) / NULLIF(nodes.cpu_total, 0), 0),
    COALESCE((nodes.memory_allocated_mb + nodes.memory_reserved_mb + sqlc.arg(memory_mb))::numeric / NULLIF(nodes.memory_total_mb, 0), 0),
    COALESCE((nodes.disk_allocated_gb + nodes.disk_reserved_gb + sqlc.arg(disk_gb))::numeric / NULLIF(nodes.disk_total_gb, 0), 0)
  ) ASC,
  nodes.weight DESC,
  nodes.id ASC
FOR UPDATE OF nodes;

-- name: AddNodeReservation :one
UPDATE nodes
SET cpu_reserved = cpu_reserved + sqlc.arg(cpu_cores),
    memory_reserved_mb = memory_reserved_mb + sqlc.arg(memory_mb),
    disk_reserved_gb = disk_reserved_gb + sqlc.arg(disk_gb),
    ipv4_reserved = ipv4_reserved + sqlc.arg(ipv4_count),
    ipv6_reserved = ipv6_reserved + sqlc.arg(ipv6_count),
    nat_port_reserved = nat_port_reserved + sqlc.arg(nat_port_count),
    version = version + 1,
    updated_at = now()
WHERE id = sqlc.arg(node_id)
  AND cpu_total - cpu_allocated - cpu_reserved >= sqlc.arg(cpu_cores)
  AND memory_total_mb - memory_allocated_mb - memory_reserved_mb >= sqlc.arg(memory_mb)
  AND disk_total_gb - disk_allocated_gb - disk_reserved_gb >= sqlc.arg(disk_gb)
  AND ipv4_total - ipv4_allocated - ipv4_reserved >= sqlc.arg(ipv4_count)
  AND ipv6_total - ipv6_allocated - ipv6_reserved >= sqlc.arg(ipv6_count)
  AND nat_port_total - nat_port_allocated - nat_port_reserved >= sqlc.arg(nat_port_count)
RETURNING *;

-- name: CreateResourceReservation :one
INSERT INTO resource_reservations (
  id, node_id, operation_id, cpu_cores, memory_mb, disk_gb,
  ipv4_count, ipv6_count, nat_port_count, status, expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'reserved', $10)
RETURNING *;

-- name: GetResourceReservationByOperation :one
SELECT * FROM resource_reservations WHERE operation_id = $1;

-- name: LockResourceReservation :one
SELECT * FROM resource_reservations WHERE id = $1 FOR UPDATE;

-- name: CommitNodeReservation :one
UPDATE nodes
SET cpu_reserved = cpu_reserved - sqlc.arg(cpu_cores),
    memory_reserved_mb = memory_reserved_mb - sqlc.arg(memory_mb),
    disk_reserved_gb = disk_reserved_gb - sqlc.arg(disk_gb),
    ipv4_reserved = ipv4_reserved - sqlc.arg(ipv4_count),
    ipv6_reserved = ipv6_reserved - sqlc.arg(ipv6_count),
    nat_port_reserved = nat_port_reserved - sqlc.arg(nat_port_count),
    cpu_allocated = cpu_allocated + sqlc.arg(cpu_cores),
    memory_allocated_mb = memory_allocated_mb + sqlc.arg(memory_mb),
    disk_allocated_gb = disk_allocated_gb + sqlc.arg(disk_gb),
    ipv4_allocated = ipv4_allocated + sqlc.arg(ipv4_count),
    ipv6_allocated = ipv6_allocated + sqlc.arg(ipv6_count),
    nat_port_allocated = nat_port_allocated + sqlc.arg(nat_port_count),
    version = version + 1,
    updated_at = now()
WHERE id = sqlc.arg(node_id)
RETURNING *;

-- name: ReleaseNodeReservation :one
UPDATE nodes
SET cpu_reserved = cpu_reserved - sqlc.arg(cpu_cores),
    memory_reserved_mb = memory_reserved_mb - sqlc.arg(memory_mb),
    disk_reserved_gb = disk_reserved_gb - sqlc.arg(disk_gb),
    ipv4_reserved = ipv4_reserved - sqlc.arg(ipv4_count),
    ipv6_reserved = ipv6_reserved - sqlc.arg(ipv6_count),
    nat_port_reserved = nat_port_reserved - sqlc.arg(nat_port_count),
    version = version + 1,
    updated_at = now()
WHERE id = sqlc.arg(node_id)
RETURNING *;

-- name: SetResourceReservationStatus :one
UPDATE resource_reservations
SET status = $2, updated_at = now()
WHERE id = $1
RETURNING *;
