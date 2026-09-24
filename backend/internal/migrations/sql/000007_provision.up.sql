ALTER TABLE plans
  ADD COLUMN default_image_id varchar(255) NOT NULL DEFAULT 'ubuntu-24.04';

ALTER TABLE subscriptions
  ADD COLUMN source_order_id uuid REFERENCES orders(id),
  ADD COLUMN source_item_index integer,
  ADD CONSTRAINT subscriptions_source_consistent CHECK (
    (source_order_id IS NULL AND source_item_index IS NULL) OR
    (source_order_id IS NOT NULL AND source_item_index > 0)
  );

CREATE UNIQUE INDEX ux_subscriptions_source_order_item
ON subscriptions(source_order_id, source_item_index)
WHERE source_order_id IS NOT NULL;

CREATE UNIQUE INDEX ux_instances_subscription ON instances(subscription_id);

ALTER TABLE instances
  ADD CONSTRAINT instances_desired_state_valid CHECK (desired_state IN ('running', 'stopped', 'suspended', 'deleted')),
  ADD CONSTRAINT instances_observed_state_valid CHECK (observed_state IN ('pending', 'provisioning', 'running', 'stopping', 'stopped', 'restarting', 'reinstalling', 'suspending', 'suspended', 'deleting', 'deleted', 'error', 'unknown'));
