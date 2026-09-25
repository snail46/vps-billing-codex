# 24 — V2 Acceptance Report

Date: 2026-09-25 (Asia/Shanghai)

## Passed on this host

- `gofmt` on changed Go packages.
- `go vet ./...`.
- `go test ./...`, including Provider contract, migration-pair, scheduler, operation,
  payment idempotency unit coverage, and exact minor-unit usage/promotion rating tests.
- `go build ./cmd/...`.
- `npm run typecheck`.
- `npm run lint` with zero warnings.
- `npm test`: 3 files / 4 tests passed, including bilingual key parity.
- `npm run build`: user and admin production bundles built.
- 19 up migrations and 19 down migrations are embedded and non-empty.
- `git diff --check` reports no whitespace errors.

## Environment-blocked release gates

These are release blockers, not waived checks:

1. `go test -race ./...` cannot build because this Windows host has no C compiler
   (`cgo: C compiler "gcc" not found`). Normal vet/test/build pass with CGO disabled.
2. No `docker`, PostgreSQL client/server, Redis, `TEST_DATABASE_URL`, or
   `DATABASE_URL` is available. PostgreSQL integration tests therefore skip and the
   Compose purchase → payment → provision → NAT → usage → renewal chain, backup/
   restore drill, and quantitative failure-injection matrix cannot execute here.

## Commands required to close V2 Acceptance

Run on a Linux/CI host with Docker and a C toolchain:

```bash
cd backend
CGO_ENABLED=1 go test -race ./...
go vet ./...
go build ./cmd/...

cd ..
npm run typecheck
npm run lint
npm test
npm run build

docker compose -f deploy/docker-compose.yml up --build --abort-on-container-exit
```

Set `TEST_DATABASE_URL` to an isolated PostgreSQL database before the backend test so
integration suites apply migrations 1–19 and exercise payment replay, concurrent
reservation, lifecycle, provision, reconciliation, and Runman persistence. Execute the
counts in `docs/23-V2-OPERATIONS-AND-FAILURE-MATRIX.md` and attach before/after database
counts. V2 Acceptance is complete only after every blocked command passes.
