#!/usr/bin/env bash
set -euo pipefail

CONTAINER="apexstock-api-optimized-1"

echo "=========================================================="
echo " [CHAOS RECOVERY] Restoring container: $CONTAINER"
echo "=========================================================="

if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER}$"; then
  docker start "$CONTAINER"
  echo ">>> Container $CONTAINER restarted successfully."
  echo ">>> NGINX health check should restore routing in ~2s."
else
  docker compose -f deploy/compose/docker-compose.yaml start api-optimized
fi
