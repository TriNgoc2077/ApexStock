#!/usr/bin/env bash
set -euo pipefail

# Decision Gate D0.12: Operator executes chaos directly via Docker CLI.
# Target: api-optimized instance (behind nginx load balancer)
CONTAINER="apexstock-api-optimized-1"

echo "=========================================================="
echo " [CHAOS] Stopping container: $CONTAINER"
echo "=========================================================="

if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER}$"; then
  docker stop "$CONTAINER"
  echo ">>> Container $CONTAINER stopped successfully."
  echo ">>> Observe War Room activity log and recovery indicator."
else
  # Fallback to docker compose service name
  echo "Container name not found, trying docker compose stop api-optimized..."
  docker compose -f deploy/compose/docker-compose.yaml stop api-optimized
fi
