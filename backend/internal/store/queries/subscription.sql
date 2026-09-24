-- name: ListSubscriptionsByUser :many
SELECT subscriptions.*, plans.slug AS plan_slug, plans.name_i18n AS plan_name_i18n
FROM subscriptions
JOIN plans ON plans.id = subscriptions.plan_id
WHERE subscriptions.user_id = $1
ORDER BY subscriptions.created_at DESC;

-- name: LockSubscriptionByUser :one
SELECT * FROM subscriptions
WHERE id = $1 AND user_id = $2
FOR UPDATE;

-- name: LockSubscriptionByID :one
SELECT * FROM subscriptions
WHERE id = $1
FOR UPDATE;

-- name: SetSubscriptionCancelAtPeriodEnd :one
UPDATE subscriptions
SET cancel_at_period_end = $2, version = version + 1, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ClaimSubscriptionsForLifecycle :many
SELECT * FROM subscriptions
WHERE (status = 'active' AND current_period_end IS NOT NULL AND current_period_end <= $1)
   OR (status = 'past_due' AND grace_until IS NOT NULL AND grace_until <= $1)
ORDER BY COALESCE(grace_until, current_period_end), id
FOR UPDATE SKIP LOCKED
LIMIT $2;

-- name: UpdateSubscriptionLifecycle :one
UPDATE subscriptions
SET status = $2,
    grace_until = $3,
    ended_at = $4,
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $5
RETURNING *;

-- name: RenewSubscriptionAfterPayment :one
UPDATE subscriptions
SET status = 'active',
    started_at = COALESCE(started_at, $2),
    current_period_start = CASE
      WHEN current_period_end IS NULL OR current_period_end <= $2 THEN $2
      ELSE current_period_start
    END,
    current_period_end = GREATEST(COALESCE(current_period_end, $2), $2) + CASE billing_cycle
      WHEN 'monthly' THEN interval '1 month'
      WHEN 'quarterly' THEN interval '3 months'
      WHEN 'yearly' THEN interval '1 year'
    END,
    next_due_at = GREATEST(COALESCE(current_period_end, $2), $2) + CASE billing_cycle
      WHEN 'monthly' THEN interval '1 month'
      WHEN 'quarterly' THEN interval '3 months'
      WHEN 'yearly' THEN interval '1 year'
    END,
    grace_until = NULL,
    cancel_at_period_end = false,
    ended_at = NULL,
    version = version + 1,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: GetSubscriptionForRenewal :one
SELECT subscriptions.*, plans.slug AS plan_slug, plans.name_i18n AS plan_name_i18n,
       products.id AS product_id, products.slug AS product_slug,
       products.name_i18n AS product_name_i18n, products.description_i18n AS product_description_i18n
FROM subscriptions
JOIN plans ON plans.id = subscriptions.plan_id
JOIN products ON products.id = plans.product_id
WHERE subscriptions.id = $1 AND subscriptions.user_id = $2
FOR UPDATE OF subscriptions;
