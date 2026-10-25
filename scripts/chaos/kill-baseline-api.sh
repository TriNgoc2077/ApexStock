#!/usr/bin/env bash
set -euo pipefail

CONTAINER="apexstock-api-baseline-1"
echo "[CHAOS] Stopping Baseline API container: $CONTAINER"
docker stop "$CONTAINER" || docker compose -f deploy/compose/docker-compose.yaml stop api-baseline
