# TASKS — V1 Implementation Roadmap

## Phase 0 — Foundation
- [x] Go module / server / worker
- [x] React+TS user-web / admin-web
- [x] shared ui / i18n / api types
- [x] PostgreSQL / Redis / Compose
- [x] health endpoints
- [x] structured logs + request_id + trace_id
- [x] migrations
- [x] CI
**Gate:** 一条 compose 命令启动基础环境。

## Phase 1 — Identity / RBAC
- [x] users/admins
- [x] independent sessions
- [x] roles/permissions
- [x] CSRF/rate limit
- [x] audit foundation
- [x] zh-CN/en-US

## Phase 2 — Commerce / Finance
- [x] products/plans
- [x] orders/order snapshots
- [x] invoices/payments
- [x] wallets/ledger
- [x] fake payment provider
- [x] transactional outbox
- [x] duplicate webhook tests
**Gate:** 同一支付回调重复 100 次只入账一次。

## Phase 3 — Subscription
- [ ] lifecycle
- [ ] renewal
- [ ] due/grace/suspend/cancel

## Phase 4 — Infrastructure Domain
- [ ] provider/node group/node
- [ ] capabilities
- [ ] reservation
- [ ] deterministic scheduler
- [ ] MockProvider

## Phase 5 — Operation System
- [ ] operations/steps
- [ ] queue/worker
- [ ] retries
- [ ] SSE
- [ ] OperationProgress component

## Phase 6 — Provision Vertical Slice
- [ ] paid order → subscription
- [ ] provision operation
- [ ] scheduler/reservation
- [ ] MockProvider create
- [ ] instance running
- [ ] notification
- [ ] E2E
**Gate:** 浏览→下单→假支付→自动开通→自动进度→运行。

## Phase 7 — Direct Provider
- [ ] CLICD 或 LXDAPI Adapter
- [ ] Contract tests
- [ ] timeout/idempotency
**Gate:** 替换 MockProvider 不修改 Business Core。

## Phase 8 — User Web
- [ ] dashboard/catalog/checkout
- [ ] instances/detail/network/traffic
- [ ] operation progress
- [ ] orders/invoices/wallet
- [ ] notifications/tickets/account
- [ ] all page states
- [ ] bilingual

## Phase 9 — Admin Web
- [ ] health dashboard
- [ ] users/products/orders/payments/ledger
- [ ] subscriptions/instances/nodes/providers
- [ ] operations/tickets/audit
- [ ] admins/roles/settings

## Phase 10 — Runman Provider
- [ ] Gateway
- [ ] auth/connection registry
- [ ] heartbeat/command/result/state
- [ ] traffic/NAT
- [ ] reconnect
- [ ] Contract tests

## Phase 11 — Reconciler / Resilience
- [ ] desired vs observed
- [ ] stuck operations
- [ ] expired reservations
- [ ] node heartbeat expiry
- [ ] create-success-but-timeout
- [ ] Redis restart / worker crash

## Phase 12 — Release Hardening
- [ ] security
- [ ] admin 2FA
- [ ] backup/restore
- [ ] metrics/alerts
- [ ] performance
- [ ] upgrade/rollback
- [ ] acceptance suite
