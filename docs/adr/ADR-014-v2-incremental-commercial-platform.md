# ADR-014 — Incremental V2 commercial platform extensions

## Status

Accepted

## Context

V1 already provides the commercial truth model, durable asynchronous execution, Provider
contract, deterministic reservations, and two web applications. V2 needs catalog,
capability-driven NAT, usage rating, richer operations, Provider operations, reliability,
and commercial workflows. Replacing those foundations would introduce duplicate truth
sources and invalidate V1 recovery guarantees.

## Decision

V2 remains a Go modular monolith with PostgreSQL, Redis, React, and the existing process
topology. New commercial facts are additive PostgreSQL aggregates and append-only Ledger
transactions. Long-running mutations extend Operation/Workflow through typed persisted
input and versioned Outbox events. NAT remains an implementation of Provider
`port_forward`; Business Core stores ownership, quota, desired status, and safe display
data only. Provider-specific configuration and branching stay inside Provider factories
and adapters. Existing `/api/v1` contracts remain compatible; additive fields and routes
are allowed.

## Consequences

- V1 deployments can roll forward through additive migrations without data conversion.
- Existing Provider adapters remain valid and can adopt normalized V2 capabilities.
- Usage rating and refunds must use Ledger rather than mutating Wallet projections.
- V2 work can ship phase-by-phase while V1 purchase, provisioning, and recovery stay live.
- The release gate must run both V1 and V2 acceptance suites.
