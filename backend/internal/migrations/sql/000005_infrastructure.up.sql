ALTER TABLE providers
  ADD CONSTRAINT providers_name_unique UNIQUE (name),
  ADD CONSTRAINT providers_status_valid CHECK (status IN ('active', 'degraded', 'disabled', 'unavailable'));

ALTER TABLE node_groups
  ADD CONSTRAINT node_groups_status_valid CHECK (status IN ('active', 'disabled'));

ALTER TABLE nodes
  ADD COLUMN ipv4_total integer NOT NULL DEFAULT 0,
  ADD COLUMN ipv4_allocated integer NOT NULL DEFAULT 0,
  ADD COLUMN ipv4_reserved integer NOT NULL DEFAULT 0,
  ADD COLUMN ipv6_total integer NOT NULL DEFAULT 0,
  ADD COLUMN ipv6_allocated integer NOT NULL DEFAULT 0,
  ADD COLUMN ipv6_reserved integer NOT NULL DEFAULT 0,
  ADD COLUMN nat_port_total integer NOT NULL DEFAULT 0,
  ADD COLUMN nat_port_allocated integer NOT NULL DEFAULT 0,
  ADD COLUMN nat_port_reserved integer NOT NULL DEFAULT 0,
  ADD CONSTRAINT nodes_status_valid CHECK (status IN ('online', 'degraded', 'draining', 'maintenance', 'offline')),
  ADD CONSTRAINT nodes_capacity_nonnegative CHECK (
    cpu_total >= 0 AND memory_total_mb >= 0 AND disk_total_gb >= 0 AND
    cpu_allocated >= 0 AND memory_allocated_mb >= 0 AND disk_allocated_gb >= 0 AND
    cpu_reserved >= 0 AND memory_reserved_mb >= 0 AND disk_reserved_gb >= 0 AND
    ipv4_total >= 0 AND ipv4_allocated >= 0 AND ipv4_reserved >= 0 AND
    ipv6_total >= 0 AND ipv6_allocated >= 0 AND ipv6_reserved >= 0 AND
    nat_port_total >= 0 AND nat_port_allocated >= 0 AND nat_port_reserved >= 0
  ),
  ADD CONSTRAINT nodes_capacity_not_oversubscribed CHECK (
    cpu_allocated + cpu_reserved <= cpu_total AND
    memory_allocated_mb + memory_reserved_mb <= memory_total_mb AND
    disk_allocated_gb + disk_reserved_gb <= disk_total_gb AND
    ipv4_allocated + ipv4_reserved <= ipv4_total AND
    ipv6_allocated + ipv6_reserved <= ipv6_total AND
    nat_port_allocated + nat_port_reserved <= nat_port_total
  );

CREATE UNIQUE INDEX ux_nodes_provider_node
ON nodes(provider_id, provider_node_id)
WHERE provider_node_id IS NOT NULL;

ALTER TABLE resource_reservations
  ADD CONSTRAINT resource_reservations_operation_unique UNIQUE (operation_id),
  ADD CONSTRAINT resource_reservations_status_valid CHECK (status IN ('reserved', 'committed', 'released', 'expired')),
  ADD CONSTRAINT resource_reservations_amounts_nonnegative CHECK (
    cpu_cores >= 0 AND memory_mb >= 0 AND disk_gb >= 0 AND
    ipv4_count >= 0 AND ipv6_count >= 0 AND nat_port_count >= 0
  );

CREATE INDEX ix_resource_reservations_expiry
ON resource_reservations(status, expires_at);
