#!/usr/bin/env bash
set -Eeuo pipefail

COMPOSE_FILE=${COMPOSE_FILE:-deploy/docker-compose.yml}
APP_URL=${APP_URL:-http://localhost:8080}
METRICS_TOKEN=${METRICS_TOKEN:?set METRICS_TOKEN}

curl --fail --show-error "$APP_URL/health/live"
curl --fail --show-error "$APP_URL/health/ready"
docker compose -f "$COMPOSE_FILE" exec -T server wget -qO- --header="Authorization: Bearer $METRICS_TOKEN" http://localhost:8080/metrics |
  grep -q 'vps_billing_metrics_collection_success 1'
curl --fail --silent --show-error --head "$APP_URL/health/live" | grep -qi '^X-Content-Type-Options: nosniff'
echo "release verification passed"
