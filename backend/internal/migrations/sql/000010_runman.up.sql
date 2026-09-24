CREATE TABLE agent_tokens (
  id uuid PRIMARY KEY, node_id uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  token_hash bytea NOT NULL UNIQUE, status varchar(32) NOT NULL DEFAULT 'active',
  created_at timestamptz NOT NULL DEFAULT now(), last_used_at timestamptz
);
CREATE TABLE agent_connections (
  node_id uuid PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE, connection_id uuid NOT NULL,
  status varchar(32) NOT NULL, remote_address text, agent_version varchar(128),
  connected_at timestamptz NOT NULL, disconnected_at timestamptz, last_heartbeat_at timestamptz
);
CREATE TABLE agent_commands (
  id uuid PRIMARY KEY, node_id uuid NOT NULL REFERENCES nodes(id), operation_id uuid REFERENCES operations(id) ON DELETE SET NULL,
  idempotency_key varchar(255) NOT NULL UNIQUE, command_type varchar(64) NOT NULL,
  payload jsonb NOT NULL, status varchar(32) NOT NULL DEFAULT 'queued', result jsonb,
  error_message text, created_at timestamptz NOT NULL DEFAULT now(), dispatched_at timestamptz,
  completed_at timestamptz, updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_agent_commands_dispatch ON agent_commands(node_id,status,created_at);
CREATE TABLE agent_messages (
  message_id varchar(255) PRIMARY KEY, node_id uuid NOT NULL REFERENCES nodes(id), received_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE agent_vm_states (
  node_id uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE, instance_id uuid NOT NULL,
  status varchar(32) NOT NULL, cpu_percent double precision NOT NULL DEFAULT 0, ram_used_mb bigint NOT NULL DEFAULT 0,
  traffic_in_bytes bigint NOT NULL DEFAULT 0, traffic_out_bytes bigint NOT NULL DEFAULT 0,
  monthly_traffic_in bigint NOT NULL DEFAULT 0, monthly_traffic_out bigint NOT NULL DEFAULT 0,
  ips jsonb NOT NULL DEFAULT '[]'::jsonb, observed_at timestamptz NOT NULL, PRIMARY KEY(node_id,instance_id)
);
CREATE TABLE agent_images (
  node_id uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE, image_id varchar(255) NOT NULL,
  name varchar(255) NOT NULL, updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(node_id,image_id)
);
CREATE TABLE agent_port_forwards (
  node_id uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE, instance_id uuid NOT NULL,
  protocol varchar(8) NOT NULL, host_port integer NOT NULL, guest_port integer NOT NULL,
  description text NOT NULL DEFAULT '', updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(node_id,instance_id,protocol,host_port)
);
