# ADR-006: Deterministic scheduler and transactional reservations

## Status

Accepted.

## Decision

V1 scheduling runs in PostgreSQL transactions. Candidates must belong to the requested active Node Group, have an active Provider, be online, satisfy JSON capability containment, and have enough unallocated and unreserved CPU, memory, disk, address, and NAT-port capacity.

Eligible candidates are ordered by the maximum projected CPU/memory/disk utilization, then descending operator weight, then UUID. The Scheduler locks the ordered candidates, chooses the first, rechecks capacity in the update, increments Node reserved counters, and inserts one Reservation per Operation in the same transaction.

Reservation commit atomically moves all resource dimensions from reserved to allocated. Release atomically subtracts reserved resources. Database checks prevent negative counters and over-subscription. Repeating the same Operation returns its existing Reservation.

Provider capabilities are data, not provider-type branches. A Provider Registry resolves Contract implementations by Provider ID. The full in-memory MockProvider is the first implementation and is used by contract and workflow tests only.

## Consequences

Scheduling is deterministic for the same committed database state. Candidate locking favors correctness over maximum placement throughput in V1. Concurrent schedulers cannot oversell capacity, and a transaction failure rolls back both counters and Reservation.

## Migration / Compatibility

Migration 000005 adds network capacity counters, state/capacity constraints, Provider/Node uniqueness, and Operation-unique Reservations. Existing counters default to zero; operators must configure total network capacity before scheduling requests that require it.

## Test Plan

Run PostgreSQL integration tests for deterministic selection, capability filtering, Operation idempotency, concurrent no-oversubscription, commit/release accounting, plus MockProvider lifecycle/idempotency/unsupported-operation tests under the race detector.
