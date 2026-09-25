# 23 — V2 Operations and Failure Matrix

V2 is an additive release on the V1 modular monolith. Migrations `000013`–`000019`
must be applied in order. A rolling deployment applies migrations first, then deploys
Server, Worker, Gateway, User Web, and Admin Web. V1 workers may run while additive
columns/tables are introduced; do not run down migrations in production.

## Worker loops and back-pressure

- Operation queue remains PostgreSQL-authoritative and Redis-delivered. Deadlines and
  heartbeat recovery prevent a claimed operation from disappearing.
- Provider health and capability checks persist bounded history and never store raw
  credentials or expose raw provider failures to users.
- Usage collection runs in five-minute buckets. `(provider_id, source_event_id)` and
  period/charge uniqueness make collection and rating replay-safe.
- Outbox delivery uses exponential backoff capped at five minutes. The tenth failed
  delivery becomes `dead_letter`; only `outbox.replay` may reset it, and replay writes
  Audit. Operators must alert on any dead letter and on oldest pending age.

## Backup, restore, and upgrade

Back up PostgreSQL and encrypted configuration before migration 13 and again before
migration 19. Restore into a new database, apply no down migration, start one Worker,
then verify `/health/ready`, pending Outbox age, operation recovery, ledger balance,
provider health, and usage period counts. Existing V1 rows receive conservative
defaults (`vps`, no setup/overage fee, no promotion, zero refunded amount).

## Failure-injection matrix

| Failure | Expected convergence | Duplicate guard |
|---|---|---|
| Payment callback repeated | One capture and one renewal extension | gateway event, external ID, Ledger reference |
| Usage sample repeated | One sample and one usage charge | provider/source event, period and charge uniqueness |
| Provider create response lost | Verify before retry; one instance | operation/provider idempotency key |
| NAT add/delete timeout | Operation retries and verifies provider mappings | one active mutation plus provider idempotency |
| Worker exits after claim | stale heartbeat requeues within retry budget | immutable attempt history |
| Redis unavailable | Outbox remains pending, then publishes | outbox event ID |
| Outbox repeatedly fails | dead-letter after ten attempts; audited replay | dead-letter state and replay RBAC |
| Agent disconnect/node offline | node/instance converge through reconciler | durable command/message IDs |
| Concurrent placement | row-locked reservation refuses oversubscription | operation reservation uniqueness |
| Refund request repeated | one refund and one balanced reversal | refund idempotency key, Ledger reference |

Release evidence must record counts before/after each injection and prove zero
duplicate financial entries, subscriptions, instances, mappings, or usage charges.
