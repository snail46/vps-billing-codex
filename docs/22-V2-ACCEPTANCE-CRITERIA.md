# 22 — V2 Acceptance Criteria

Current execution evidence and unresolved release gates are recorded in
`docs/24-V2-ACCEPTANCE-REPORT.md`.

V2 extends V1; every V1 release blocker remains a V2 release blocker. The V2 gate is
evidence-based: a feature is not complete until its database migration, API contract,
UI states, authorization, Audit/metrics where applicable, automated tests, and operator
documentation agree.

## Product acceptance

- A user can register, compare an available VPS or NAT VPS plan, create an order, pay,
  follow provisioning, manage the instance, manage allowed NAT mappings, inspect usage,
  renew, and retrieve invoices without knowing the Provider implementation.
- An administrator can operate catalog, users, finance, subscriptions, instances,
  Providers, nodes, Operations, usage, support, RBAC, and settings from the admin entry.
- Every visible surface supports loading, loaded, empty, error, retry, progress where
  relevant, partial error for composed views, and permission denied for protected views.
- All user-visible copy has matching `zh-CN` and `en-US` keys.

## Integrity acceptance

- Order, Payment, Invoice, Subscription, Instance, Operation, and Ledger remain separate.
- Prices, discounts, tax inputs, and rated usage are snapshotted; later catalog changes
  cannot rewrite an existing financial fact.
- Payment, refund, wallet, and usage charges are balanced Ledger transactions. Ledger
  history is append-only.
- Replaying any payment webhook, usage sample, Operation message, Agent result, or
  Provider create request 100 times produces one corresponding business fact.
- NAT is a Provider capability. There is no platform NAT controller and no direct
  frontend-to-Provider path.

## Async and recovery acceptance

- Create/delete/power/reinstall/password/NAT mutations return HTTP 202 with an Operation
  ID within one second, then run through Worker/Workflow/Provider.
- Every Operation exposes real step-derived progress, stable error code, retry state,
  trace ID, and an authorized recovery query. Raw Provider errors remain admin-only.
- Create timeout verifies before retry. Worker crash, Redis restart, Provider timeout,
  Agent disconnect, and node offline scenarios converge without duplicate instances or
  permanent task loss.

## Release evidence

- Database migrations are paired, forward compatible for rolling deploys, and reflected
  in `db/schema.sql`.
- OpenAPI and TypeScript API types match runtime responses.
- `gofmt`, `go vet`, `go test -race ./...`, backend builds, frontend lint/tests/typecheck/
  builds, migration tests, Provider contract tests, and Compose acceptance all pass.
- The V2 failure matrix covers at least: 100 complete purchases, 100 duplicate payment
  callbacks, 100 duplicate usage samples, 100 restarts, 50 reinstalls, 50 Provider
  timeouts, 20 worker crashes, 20 Redis restarts, 20 Agent disconnects, 20 node-offline
  transitions, and 20 duplicate creates, with zero V1/V2 release blockers.
