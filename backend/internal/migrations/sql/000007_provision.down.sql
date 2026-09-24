ALTER TABLE instances
  DROP CONSTRAINT IF EXISTS instances_observed_state_valid,
  DROP CONSTRAINT IF EXISTS instances_desired_state_valid;

DROP INDEX IF EXISTS ux_instances_subscription;
DROP INDEX IF EXISTS ux_subscriptions_source_order_item;

ALTER TABLE subscriptions
  DROP CONSTRAINT IF EXISTS subscriptions_source_consistent,
  DROP COLUMN IF EXISTS source_item_index,
  DROP COLUMN IF EXISTS source_order_id;

ALTER TABLE plans DROP COLUMN IF EXISTS default_image_id;
