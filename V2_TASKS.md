# TASKS — V2 Commercial Platform Roadmap

V2 is an incremental release on top of the V1 modular monolith. The V1 Billing Core,
Ledger, Operation/Workflow engine, Provider layer, state machines, and reconciliation
rules remain authoritative.

## Phase 1 — User Web productization
- [x] Purchase flow continues from order creation to payment/provision status
- [x] Subscription management is a first-class page
- [x] Instance detail exposes capability-driven networking and usage
- [x] Responsive navigation, accessible dialogs, retry and partial-error states
- [x] `zh-CN` / `en-US` parity and frontend acceptance tests

## Phase 2 — Admin Web productization
- [x] Search, filtering, pagination, and resource details
- [x] Product/plan lifecycle management
- [x] Provider/node health and capacity operations
- [x] Operation retry/cancel with explicit Audit
- [x] Permission-denied and partial-error states

## Phase 3 — Product Catalog
- [x] VPS/NAT VPS product classification and regional availability
- [x] Capability/stock-aware catalog responses
- [x] Versioned price snapshots remain authoritative for orders
- [x] Admin product/plan API, Audit, validation, and tests

## Phase 4 — NAT VPS Support
- [x] Provider capability keys `shared_ipv4`, `port_forward`, `traffic_meter`
- [x] User-owned port-forward list/add/delete APIs
- [x] NAT mutations use Operation + Workflow + Provider and return HTTP 202
- [x] Port quota/conflict handling, Audit/diagnostics, UI, and contract tests

## Phase 5 — Operation Engine 2.0
- [x] Typed operation input and safe public result
- [x] Admin retry/cancel controls with immutable attempt history
- [x] Deadline, heartbeat, retry policy, and stable error taxonomy
- [x] User/admin timelines recover through SSE plus polling

## Phase 6 — Provider Platform
- [x] Capability normalization and inventory/health synchronization
- [x] Admin provider detail with secret-safe configuration
- [x] Provider health history and capability drift visibility
- [x] Contract conformance suite for every registered adapter

## Phase 7 — Scheduler
- [x] Deterministic capability, region, maintenance, and capacity filtering
- [x] Configurable placement policy with stable tie-breaking
- [x] Placement decision diagnostics without leaking secrets
- [x] Concurrent reservation and anti-oversubscription acceptance tests

## Phase 8 — Usage Billing
- [x] Idempotent usage ingestion and aggregation
- [x] Billing periods, included allowance, and minor-unit rating
- [x] Ledger-backed usage charges and invoice line items
- [x] User usage view and admin usage/revenue diagnostics

## Phase 9 — Reliability
- [x] Dead-letter visibility and controlled replay
- [x] Reconciliation SLOs, back-pressure, and health history
- [x] Backup/restore and upgrade compatibility for V2 schema
- [ ] Failure-injection acceptance matrix

## Phase 10 — Commercial Features
- [x] Coupons/promotions with immutable order snapshots
- [x] Tax/profile-ready invoice fields
- [x] Refund workflow with Payment + Ledger + Audit transactionality
- [x] Dunning notifications and renewal recovery

## V2 Acceptance Gate
- [ ] All phase checks above are complete
- [ ] V1 acceptance remains green
- [ ] V2 API schema, database schema, docs, and UI agree
- [ ] Format, lint, unit, integration, race, typecheck, and builds pass
- [ ] Compose exercises purchase → payment → provision → NAT → usage → renewal
- [ ] Repeated payment/usage/provider messages create no duplicate financial or infrastructure facts
- [ ] No authorization bypass, permanent task loss, silent failure, or secret exposure
