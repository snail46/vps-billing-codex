# ADR-013 — Durable Reconciliation and Worker Recovery

Status: Accepted  
Date: 2026-09-24

## Context

Operations can outlive an HTTP request, Worker process, Redis connection, or Agent session. Redis delivery alone cannot prove completion, and blindly repeating Provider Create can duplicate infrastructure. Desired subscription state can also diverge from observed Provider state.

## Decision

PostgreSQL remains the recovery authority. A Reconciler Worker processor:

- requeues operations with stale heartbeats and creates a new transactional outbox event;
- expires reservations only after their operation is terminal;
- marks stale Agent nodes offline and their instances unknown, never deleted;
- refreshes provider observed state and converts actionable drift into an idempotent Operation.

Redis Stream pending entries are reclaimed with XAUTOCLAIM. Outbox rows remain pending when Redis is unavailable and are republished after recovery. Provider Create retries retain the same platform Instance ID and idempotency key, so a success whose response was lost is verified rather than duplicated.

Subscription suspension and recovery update desired state only. Provider Stop/Start remains inside the registered Suspend/Start Workflow and Operation progress remains visible through the existing SSE/polling APIs.

## Consequences

- Redis is delivery infrastructure, not the durable task source.
- Worker replacement is automatic after the stale-heartbeat threshold.
- Offline Agents produce unknown observed state rather than destructive inference.
- Reconciliation has bounded batches and Provider read timeouts.
- Operations that exhaust retry budget fail explicitly and retain error/trace history.
