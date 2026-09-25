DROP TABLE IF EXISTS scheduler_decisions;
ALTER TABLE node_groups
  DROP CONSTRAINT IF EXISTS node_groups_placement_policy_valid,
  DROP COLUMN IF EXISTS placement_policy;
