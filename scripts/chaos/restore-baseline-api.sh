#!/usr/bin/env bash
set -euo pipefail

CONTAINER="apexstock-api-baseline-1"
echo "[CHAOS RECOVERY] Restoring Baseline API container: $CONTAINER"
docker start "$CONTAINER" || docker compose -f deploy/compose/docker-compose.yaml start api-baseline
