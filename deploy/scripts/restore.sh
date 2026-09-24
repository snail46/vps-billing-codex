#!/usr/bin/env bash
set -Eeuo pipefail

if [ "${1:-}" != "--confirm" ]; then
  echo "restore is destructive; rerun with --confirm" >&2
  exit 2
fi
: "${BACKUP_SET:?set BACKUP_SET to a backup directory}"
: "${BACKUP_PASSPHRASE_FILE:?set BACKUP_PASSPHRASE_FILE to the matching passphrase file}"
: "${CONFIG_RESTORE_DIRECTORY:?set CONFIG_RESTORE_DIRECTORY to an empty restricted directory}"

COMPOSE_FILE=${COMPOSE_FILE:-deploy/docker-compose.yml}
POSTGRES_SERVICE=${POSTGRES_SERVICE:-postgres}
POSTGRES_DB=${POSTGRES_DB:-vps_billing}
POSTGRES_USER=${POSTGRES_USER:-vps_billing}

(cd "$BACKUP_SET" && sha256sum -c SHA256SUMS)
mkdir -p "$CONFIG_RESTORE_DIRECTORY"
if [ -n "$(ls -A "$CONFIG_RESTORE_DIRECTORY")" ]; then
  echo "CONFIG_RESTORE_DIRECTORY must be empty" >&2
  exit 2
fi
openssl enc -d -aes-256-cbc -pbkdf2 -pass "file:$BACKUP_PASSPHRASE_FILE" -in "$BACKUP_SET/config.tar.enc" |
  tar -xf - -C "$CONFIG_RESTORE_DIRECTORY"
openssl enc -d -aes-256-cbc -pbkdf2 -pass "file:$BACKUP_PASSPHRASE_FILE" -in "$BACKUP_SET/database.dump.enc" |
  docker compose -f "$COMPOSE_FILE" exec -T "$POSTGRES_SERVICE" \
    pg_restore --clean --if-exists --no-owner --no-privileges --exit-on-error --username "$POSTGRES_USER" --dbname "$POSTGRES_DB"

echo "database and configuration restored; review the isolated configuration, then run migrations and readiness checks"
