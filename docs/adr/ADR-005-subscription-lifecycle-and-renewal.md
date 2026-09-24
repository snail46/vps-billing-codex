# ADR-005: Subscription lifecycle and renewal settlement

## Status

Accepted.

## Decision

Subscription remains separate from Order, Payment, Invoice, and Instance. A renewal request creates a new renewal Order, Invoice, and pending Payment linked to the target Subscription. The Subscription's snapshotted price, currency, and billing cycle are authoritative for that renewal; the client cannot submit a price.

When the renewal Payment succeeds, the existing financial transaction also extends the Subscription period and writes `subscription.renewed.v1` to the transactional Outbox. The gateway event receipt makes both the Ledger posting and period extension idempotent under concurrent duplicate callbacks. Renewal can restore `past_due` or `suspended` subscriptions to `active`, but terminal subscriptions reject settlement.

A recoverable Worker processor owns time-based transitions. At period end an active subscription either becomes `past_due` with a deterministic grace deadline or becomes `cancelled` when cancellation was scheduled. At the grace deadline `past_due` becomes `suspended`. Every transition and cancellation scheduling change writes a versioned Outbox event in the same database transaction.

## Consequences

Lifecycle time is based on persisted period boundaries rather than worker wake-up time. Worker downtime can delay observation, but cannot extend the contractual grace period. Phase 3 changes only commercial entitlement state; Provider-side suspend/unsuspend is introduced later through Operation and Workflow.

## Migration / Compatibility

Migration 000004 constrains supported lifecycle states and billing cycles, indexes lifecycle deadlines, and adds typed purchase/renewal linkage to Orders. Existing Orders default to `purchase` and remain compatible.

## Test Plan

Verify monthly renewal, duplicate callback concurrency, Ledger/Outbox uniqueness, due-to-grace-to-suspend transitions, scheduled cancellation, ownership, idempotency-key conflicts, migration generation, and the Compose Gate.
