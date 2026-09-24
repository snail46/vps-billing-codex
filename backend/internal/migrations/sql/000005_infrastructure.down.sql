DROP INDEX IF EXISTS ix_resource_reservations_expiry;
ALTER TABLE resource_reservations
  DROP CONSTRAINT IF EXISTS resource_reservations_amounts_nonnegative,
  DROP CONSTRAINT IF EXISTS resource_reservations_status_valid,
  DROP CONSTRAINT IF EXISTS resource_reservations_operation_unique;

DROP INDEX IF EXISTS ux_nodes_provider_node;
ALTER TABLE nodes
  DROP CONSTRAINT IF EXISTS nodes_capacity_not_oversubscribed,
  DROP CONSTRAINT IF EXISTS nodes_capacity_nonnegative,
  DROP CONSTRAINT IF EXISTS nodes_status_valid,
  DROP COLUMN IF EXISTS nat_port_reserved,
  DROP COLUMN IF EXISTS nat_port_allocated,
  DROP COLUMN IF EXISTS nat_port_total,
  DROP COLUMN IF EXISTS ipv6_reserved,
  DROP COLUMN IF EXISTS ipv6_allocated,
  DROP COLUMN IF EXISTS ipv6_total,
  DROP COLUMN IF EXISTS ipv4_reserved,
  DROP COLUMN IF EXISTS ipv4_allocated,
  DROP COLUMN IF EXISTS ipv4_total;

ALTER TABLE node_groups DROP CONSTRAINT IF EXISTS node_groups_status_valid;
ALTER TABLE providers
  DROP CONSTRAINT IF EXISTS providers_status_valid,
  DROP CONSTRAINT IF EXISTS providers_name_unique;
