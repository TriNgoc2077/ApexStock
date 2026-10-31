#!/usr/bin/env bash
set -euo pipefail

# Dynamic container name resolution (supports both compose- and apexstock- prefixes)
CONTAINER=$(docker ps --format '{{.Names}}' | grep 'api-optimized' | head -n 1 || echo "")

if [ -z "$CONTAINER" ]; then
  echo "Error: No running api-optimized container found!" >&2
  exit 1
fi

echo "=========================================================="
echo " [CHAOS] Stopping 1 replica: $CONTAINER"
echo "=========================================================="

docker stop "$CONTAINER"
echo ">>> Container $CONTAINER stopped successfully."
echo ">>> Other 2 replicas are still active behind NGINX."
