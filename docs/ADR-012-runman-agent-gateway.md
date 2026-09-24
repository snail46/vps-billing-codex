# ADR-012 — Runman Agent Gateway

Status: Accepted  
Date: 2026-09-24

## Context

Runman Agent initiates a long-lived bidirectional gRPC connection, while Provider workflows execute in separate Worker processes. Treating it as a synchronous HTTP Provider would lose commands across disconnects and couple Business Core to protocol messages.

## Decision

Runman remains a Provider Adapter behind the existing Provider Contract. Worker persists every command in PostgreSQL with an idempotency key and waits on its durable result. A separate Gateway authenticates hashed bearer tokens, registers the current connection, receives heartbeats, and dispatches queued commands. Reconnect replays queued/dispatched commands with the original command_id; Agent-side command idempotency and platform message_id deduplication make replay safe.

The platform Instance UUID is the protocol vm_id. PostgreSQL is the recovery source; the in-memory connection registry is only a delivery optimization. Production gRPC requires TLS. Client timestamps are informational: availability uses server receipt time.

## Consequences

- Gateway and Worker scale independently without changing Business Core.
- A committed command survives Redis, Worker, Gateway, and network restarts.
- Agent implementations must durably deduplicate command_id as required by the upstream protocol.
- Command payloads needed for reconnect are retained until terminal completion; password fields are then removed.
- Protocol upgrades are isolated to the Gateway/Adapter and regenerated protobuf files.
