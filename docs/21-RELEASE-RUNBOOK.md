# 21 — V1 Release Runbook

## Preconditions

- Use immutable image tags and retain the previous tag.
- Set distinct 32+ character application secrets and `METRICS_TOKEN` outside source control.
- Production requires HTTPS origins, `COOKIE_SECURE=true`, `FAKE_PAYMENT_ENABLED=false`, and TLS for Runman and Direct Providers.
- Confirm PostgreSQL and Redis persistence, backup capacity, alert routing, and at least one healthy Worker.

## Backup and restore drill

Set `BACKUP_DIRECTORY`, `BACKUP_PASSPHRASE_FILE`, and `CONFIG_SOURCE`, then run `deploy/scripts/backup.sh`. The command creates encrypted database and configuration archives plus SHA-256 checksums. Keep the passphrase in a secret manager, separate from the backup.

Quarterly, restore into an isolated empty environment with `BACKUP_SET`, an empty restricted `CONFIG_RESTORE_DIRECTORY`, and `deploy/scripts/restore.sh --confirm`. Review the decrypted configuration before use. Run migrations, `deploy/scripts/verify-release.sh`, ledger-balance queries, and a purchase/provision smoke test. Never test restore against production.

## Upgrade

1. Put the release into a change window and confirm the previous image tag is available.
2. Create and verify an encrypted backup.
3. Pull/build the candidate images without stopping the current stack.
4. Run the candidate `/app/migrate` once. Migrations are forward-only in production and must be backward compatible with the prior application during rollout.
5. Roll server, Worker, Gateway, and web containers; keep at least one Worker active.
6. Run `deploy/scripts/verify-release.sh`, then verify payment idempotency, Operation recovery, Provider health, queue/outbox depth, stale heartbeat, capacity, and error-rate alerts.

## Rollback

- If schema remains backward compatible, redeploy the previous immutable application images and repeat release verification. Do not run down migrations in production.
- If data/schema restoration is required, stop all writers, preserve the failed-state database for investigation, create a fresh database, and restore the pre-upgrade backup. Reapply only migrations belonging to the selected release, then verify before reopening traffic.
- Any payment, Ledger, duplicate-instance, authorization, or permanent-task-loss symptom is a release blocker and requires traffic closure plus incident review.

## Alerts

Alert before automatic recovery thresholds: Operation heartbeat age approaching 120 seconds and Agent heartbeat age approaching 90 seconds. Also alert on pending outbox growth, failed Operations, Provider/Node offline state, metrics collection failure, HTTP 5xx rate, low capacity, backup failure, and restore-drill failure.
