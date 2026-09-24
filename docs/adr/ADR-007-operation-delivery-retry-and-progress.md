# ADR-007 — Operation delivery, retry, and progress

## Status

Accepted.

## Decision

Long-running work is represented by a durable Operation and ordered OperationSteps. Creation persists the Operation, steps, and `operation.queued.v1` Outbox event in one PostgreSQL transaction. The Outbox dispatcher publishes all domain events to `domain-events` and queued operations to the Redis Stream `operation-queue`. Workers use a consumer group and may execute only after an atomic queued-to-running database claim.

Retryable workflow errors persist a bounded retry count and `next_attempt_at`. A database-backed RetryScheduler creates a new queued Outbox event when due. Delivery is at least once; duplicate queue records are harmless because only queued operations can be claimed. Exponential backoff is capped. Phase 11 adds abandoned-consumer recovery and stale-heartbeat reconciliation.

Authenticated UI clients receive user-owned events over SSE with `Last-Event-ID` resume support. Polling `GET /operations/{id}` remains the authoritative recovery path. Public responses contain stable error codes and message keys; raw diagnostic errors remain server-side.

## Consequences

Redis is delivery infrastructure rather than the source of truth. A Redis outage delays work but does not lose the Operation. Publishing the domain stream and operation stream is not atomic, so duplicates are possible and consumers must remain idempotent.

## Compatibility and migration

Migration 000006 adds Operation ownership, retry scheduling, heartbeat, constraints, indexes, and safe step output. Existing rows receive an immediately eligible next-attempt timestamp and remain actorless system operations.

## Verification

Run unit tests for SSE ownership and integration tests covering concurrent idempotent creation, Outbox delivery, retry scheduling, recovery, terminal completion, and cross-user isolation.
