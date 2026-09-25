ALTER TABLE node_groups
  ADD COLUMN placement_policy varchar(32) NOT NULL DEFAULT 'spread',
  ADD CONSTRAINT node_groups_placement_policy_valid CHECK (placement_policy IN ('spread','pack'));

CREATE TABLE scheduler_decisions (
  id uuid PRIMARY KEY,
  operation_id uuid NOT NULL REFERENCES operations(id),
  node_group_id uuid NOT NULL REFERENCES node_groups(id),
  selected_node_id uuid REFERENCES nodes(id),
  placement_policy varchar(32) NOT NULL,
  candidate_count integer NOT NULL CHECK (candidate_count >= 0),
  requested_resources jsonb NOT NULL,
  required_capabilities jsonb NOT NULL,
  selected_score numeric(12,6),
  result varchar(32) NOT NULL CHECK (result IN ('selected','exhausted')),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ix_scheduler_decisions_operation ON scheduler_decisions(operation_id, created_at DESC);
CREATE INDEX ix_scheduler_decisions_group ON scheduler_decisions(node_group_id, created_at DESC);
