.PHONY: help build test lint docker-up docker-down clean db-migrate db-seed db-warmup db-reset db-reconcile

help:
	@echo "ApexStock — Makefile targets:"
	@echo "  build         Build all Go binaries"
	@echo "  test          Run Go tests with race detector"
	@echo "  lint          Run gofmt and go vet"
	@echo "  docker-up     Start Docker Compose (dev profile)"
	@echo "  docker-down   Stop Docker Compose"
	@echo "  db-migrate    Run PostgreSQL migrations (both DBs)"
	@echo "  db-seed       Seed deterministic test data"
	@echo "  db-warmup     Warm up Redis from PostgreSQL"
	@echo "  db-reset      Reset databases and Redis"
	@echo "  db-reconcile  Run SQL reconciliation checks"
	@echo "  clean         Remove binaries and build artifacts"

build:
	@echo "Building Go binaries..."
	go build -o bin/api-baseline ./cmd/api-baseline
	go build -o bin/api-optimized ./cmd/api-optimized
	go build -o bin/worker ./cmd/worker
	go build -o bin/warroom-controller ./cmd/warroom-controller

test:
	@echo "Running Go tests..."
	go test -race ./...

lint:
	@echo "Running gofmt and go vet..."
	gofmt -l . | tee /dev/stderr | (! grep .)
	go vet ./...

docker-up:
	@echo "Starting Docker Compose (dev profile)..."
	docker compose -f deploy/compose/docker-compose.yaml --profile dev up -d

docker-down:
	@echo "Stopping Docker Compose..."
	docker compose -f deploy/compose/docker-compose.yaml down

clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f cmd/*/api-baseline cmd/*/api-optimized cmd/*/worker cmd/*/warroom-controller

db-migrate:
	@echo "Running PostgreSQL migrations..."
	@./scripts/migrate.sh

db-seed:
	@echo "Seeding deterministic test data..."
	@./scripts/seed-data.sh

db-warmup:
	@echo "Warming up Redis from PostgreSQL..."
	@./scripts/warmup-redis.sh

db-reset:
	@echo "Resetting databases and Redis..."
	@./scripts/reset-data.sh

db-reconcile:
	@echo "Running SQL reconciliation checks..."
	@./scripts/reconcile.sql.sh
