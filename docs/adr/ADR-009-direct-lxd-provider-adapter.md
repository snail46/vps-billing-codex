# ADR-009 — Direct LXD provider adapter

## Status

Accepted.

## Context

V1 requires one production-oriented direct Provider without exposing vendor fields to the Business Core. Canonical LXD publishes a stable `/1.0` REST API, asynchronous operations, project isolation, cluster targeting, and mutual-TLS authentication.

The implementation was checked on 2026-09-23 against the current Canonical LXD documentation and source for instance creation, rebuild, remote authentication, projects, operations, and client interfaces.

## Decision

Implement `lxdapi` behind the existing Provider Contract. The Adapter uses only LXD REST endpoints and translates contract requests, state, operations, and errors at the provider boundary. The Worker registers its factory alongside MockProvider; workflow and domain packages remain unchanged.

Production endpoints must use HTTPS with server verification and client certificates. `credential_ref` resolves certificate, key, and CA values from process environment; secrets are never stored in provider configuration. Plain HTTP is available only through the test constructor and cannot be enabled by persisted configuration.

Create uses a deterministic `vps-<platform-instance-id>` name and writes platform instance, operation, and idempotency identifiers into LXD instance configuration. A retry verifies that identity before returning success. Mutating actions use action-scoped idempotency keys and reject key reuse for another instance. LXD asynchronous responses are followed through the operation wait endpoint with a bounded timeout.

Reset-password and NAT port-forward operations are not portable LXD server capabilities, so this Adapter advertises them as unsupported and returns `UNSUPPORTED_OPERATION`.

## Consequences

Selecting `provider_type=lxdapi` replaces MockProvider without Business Core changes. Projects and cluster member targets remain provider-layer configuration. Adapter-local action replay records do not replace durable platform Operation idempotency; after a Worker restart, naturally convergent LXD state actions can be repeated while create remains durable through instance identity metadata.

## Compatibility and migration

No schema or public API migration is required. Existing MockProvider records continue to work. LXD Provider rows require an HTTPS endpoint, a credential reference, and optional JSON configuration for project, image server, and operation timeout.

## Verification

Contract tests cover health, capabilities, images, create/get, state actions, reinstall, delete, usage, traffic, unsupported capabilities, project and cluster targeting, duplicate requests, conflicting idempotency keys, transport timeout, and normalized provider errors.
