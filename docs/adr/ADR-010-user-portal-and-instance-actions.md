# ADR-010 — User portal API and serialized instance actions

## Status

Accepted.

## Decision

The user web reads platform-owned projections through authenticated `/api/v1` endpoints. Instance, network, traffic, notification, and ticket ownership is enforced by repository queries that always join the resource to the authenticated user. The browser never calls a Provider and never treats Provider data as commercial truth.

Start, stop, restart, and reinstall use the existing Operation delivery path: HTTP returns `202` with an Operation ID, Outbox publishes to the Redis Stream, Worker executes a registered Workflow through the Provider Contract, and the UI follows progress using SSE with polling fallback. A partial unique index permits only one active instance action per instance while allowing a new action after the earlier Operation reaches a terminal state.

Reinstall accepts only the instance's currently selected image in this phase. Image catalog selection can be extended without putting vendor fields in the public API.

The user web uses a small hash-based navigation shell to avoid adding a router dependency for V1. Every data surface has loading, empty, error, permission-denied-compatible error rendering, and the dashboard/detail surfaces preserve usable data during partial failures. All visible labels and statuses use shared `zh-CN` and `en-US` message catalogs.

## Consequences

User interactions receive immediate feedback and survive refresh through durable Operation queries. Repeated clicks with different idempotency keys cannot run concurrent destructive actions. Tickets and notifications are platform records, not static UI fixtures.

## Compatibility and migration

Migration 000008 adds the active instance-action unique index and validates ticket status/priority values. Existing valid tickets and Operations are compatible. New APIs are additive.

## Verification

Backend unit/integration tests validate migration and Operation serialization. Frontend lint, typecheck, bilingual-catalog parity, tests, and production builds run in CI. Compose acceptance covers instance list/detail/network/traffic, notifications, ticket creation/reply, a restart Operation, SSE/polling-compatible Operation retrieval, and the existing purchase/provision flow.
