#!/usr/bin/env bash
set -Eeuo pipefail

: "${BACKUP_DIRECTORY:?set BACKUP_DIRECTORY to a restricted directory}"
: "${BACKUP_PASSPHRASE_FILE:?set BACKUP_PASSPHRASE_FILE to a root-readable passphrase file}"
: "${CONFIG_SOURCE:?set CONFIG_SOURCE to the deployment configuration file or directory}"

COMPOSE_FILE=${COMPOSE_FILE:-deploy/docker-compose.yml}
POSTGRES_SERVICE=${POSTGRES_SERVICE:-postgres}
POSTGRES_DB=${POSTGRES_DB:-vps_billing}
POSTGRES_USER=${POSTGRES_USER:-vps_billing}
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
destination="$BACKUP_DIRECTORY/$timestamp"
umask 077
mkdir -p "$destination"

docker compose -f "$COMPOSE_FILE" exec -T "$POSTGRES_SERVICE" \
  pg_dump --format=custom --no-owner --no-privileges --username "$POSTGRES_USER" "$POSTGRES_DB" |
  openssl enc -aes-256-cbc -salt -pbkdf2 -pass "file:$BACKUP_PASSPHRASE_FILE" -out "$destination/database.dump.enc"

config_parent=$(dirname "$CONFIG_SOURCE")
config_name=$(basename "$CONFIG_SOURCE")
tar -C "$config_parent" -cf - "$config_name" |
  openssl enc -aes-256-cbc -salt -pbkdf2 -pass "file:$BACKUP_PASSPHRASE_FILE" -out "$destination/config.tar.enc"

(
  cd "$destination"
  sha256sum database.dump.enc config.tar.enc > SHA256SUMS
)
printf '%s\n' "$destination"
