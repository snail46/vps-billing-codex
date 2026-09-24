# ADR-004: Financial transaction and ledger boundary

## Status

Accepted.

## Decision

Order, Payment, Invoice, Wallet, and Ledger remain separate domain records. Prices are read from an active Plan and copied into immutable order and invoice snapshots; clients never submit authoritative prices. Money is represented as signed 64-bit minor units plus an ISO-style three-letter currency.

A successful payment callback is processed under row locks in one PostgreSQL transaction. That transaction inserts an idempotent webhook receipt, changes Payment, Order, and Invoice state, writes a balanced two-entry Ledger transaction, and appends `payment.succeeded.v1` to the transactional Outbox. Unique webhook, external payment, and Ledger reference constraints provide defense in depth. Ledger rows are database-immutable; corrections must use new adjustment transactions.

The Outbox worker publishes versioned envelopes to a Redis stream. Delivery is at least once, so consumers must deduplicate by `event_id`. A publish-before-commit crash can create a duplicate message but cannot lose the database event.

## Consequences

Payment callbacks validate signature, event identity, amount, currency, payment identity, and current state. Repeating the same callback, including concurrently, produces one financial transaction and one outbox event. Redis availability is not part of the payment commit path; delayed events remain visible and retryable in PostgreSQL.
