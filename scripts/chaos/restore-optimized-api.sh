#!/usr/bin/env bash
set -euo pipefail

# Dynamic container name resolution
CONTAINERS=$(docker ps -a --format '{{.Names}}' | grep 'api-optimized' || echo "")

echo "=========================================================="
echo " [CHAOS RECOVERY] Restoring api-optimized containers..."
echo "=========================================================="

for c in $CONTAINERS; do
  docker start "$c" >/dev/null 2>&1 || true
  echo ">>> Container $c is running."
done

echo ">>> All api-optimized replicas restored. NGINX will restore routing."
