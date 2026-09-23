# ADR-008 — Event-driven provision transaction boundaries

## Status

Accepted.

## Decision

The payment webhook transaction remains the financial boundary: Payment, Order, Invoice, Ledger, webhook receipt, and `payment.succeeded.v1` Outbox are committed together. Provider calls never occur inside that transaction.

A dedicated consumer reacts only to successful purchase payments. In one PostgreSQL transaction it idempotently creates one Subscription and Instance per purchased quantity, a Provision Operation with ordered steps, and its queued Outbox event. `(source_order_id, source_item_index)` and the Operation idempotency key make duplicate event delivery safe.

The Provision Workflow reserves capacity through the Scheduler, resolves an Adapter through the Provider Registry, creates and verifies the instance, persists observed state, then finalizes. Finalization atomically moves reserved capacity to allocated, activates the Subscription, completes the Order only when all purchased subscriptions are active, creates the user Notification, and writes activation/running Outbox events.

## Consequences

Payment success is not blocked by infrastructure availability. A purchase can remain paid/fulfilling with a recoverable failed or retrying Operation. PostgreSQL remains authoritative and Redis delivery is at least once. Provider-specific factory selection stays in the provider/composition layer.

## Compatibility and migration

Migration 000007 adds Plan default image configuration, purchase source identity on Subscription, one-Instance-per-Subscription uniqueness, and Instance state constraints. Existing plans receive the V1 development image default and can be updated before enabling another Adapter.

## Verification

Run PostgreSQL/Redis integration tests and the Compose journey: register, order, duplicate payment callbacks, automatic provision, authorized Operation polling, running Instance, active Subscription, committed Reservation, fulfilled Order, and one ready Notification.
