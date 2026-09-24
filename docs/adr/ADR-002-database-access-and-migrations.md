# ADR-002 — Database Access and Versioned Migrations

Status: Accepted
Date: 2026-09-23

## Context

PostgreSQL is the commercial source of truth. Schema evolution and query behavior must be reviewable, reproducible, and free from ORM auto-migration behavior.

## Decision

Use pgx/v5 for connections, sqlc for generated typed query code, and golang-migrate with embedded paired `up`/`down` SQL files. Production schema changes are append-only migration files. Business transactions are coordinated explicitly by application services.

## Alternatives

ORM auto-migration and runtime schema inference were rejected because they obscure SQL and make financial transaction review harder. The original custom migration runner was replaced with the fixed project toolchain.

## Consequences

Generated sqlc code is committed and CI verifies regeneration. Migration rollback is explicit and must be exercised before release. Ledger history remains immutable regardless of schema tooling.

## Migration / Compatibility

The original initial schema becomes migration `000001_initial`. Existing databases at version 1 remain compatible with the same `schema_migrations` version record.

## Test Plan

Validate migration discovery, apply migrations against PostgreSQL in Compose, run sqlc generation without a diff, and verify readiness after migration.
