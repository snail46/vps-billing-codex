# ADR-001 — V1 Modular Monolith Technology Stack

Status: Accepted
Date: 2026-09-23

## Context

V1 needs clear domain boundaries and independently runnable API and worker processes without the operational cost of distributed services.

## Decision

Use one Go 1.25 module as a modular monolith. The API uses chi, PostgreSQL through pgx/v5 and sqlc-generated repositories, Redis for queue coordination, and separate `server`, `worker`, and `migrate` processes. User and admin experiences remain separate React applications built with Vite, TypeScript, Tailwind CSS, TanStack Query, and i18next.

## Alternatives

Microservices were rejected because they add deployment and consistency costs before V1 domain boundaries are proven. A single process was rejected because worker failures and HTTP lifecycle concerns need isolation.

## Consequences

Domain packages must not bypass application services. Both processes share domain and infrastructure code while retaining separate lifecycles. Scaling is per process, not per internal module.

## Migration / Compatibility

The Phase 0 standard-library router and custom frontend context are replaced without changing published health URLs or response envelopes.

## Test Plan

Run Go unit/race tests, frontend lint/typecheck/tests/build, and the complete Docker Compose health Gate.
