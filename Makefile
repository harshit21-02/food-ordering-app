# Cafe Ordering — dev workflow
#
# All migrate/seed/test targets read DATABASE_URL from `.env` (via godotenv in
# the Go programs themselves) so they work against any Postgres — local Docker
# or AWS RDS / Supabase / Neon / etc. Just point DATABASE_URL at the cloud and
# everything below works unchanged.

.PHONY: help db-up db-down db-logs db-shell migrate-up migrate-down migrate-reset migrate-version seed backend frontend test test-db-create test-db-drop

help:
	@echo "Database (local Docker):"
	@echo "  make db-up           start postgres in the background"
	@echo "  make db-down         stop postgres (data preserved)"
	@echo "  make db-logs         tail postgres logs"
	@echo "  make db-shell        open psql inside the container"
	@echo ""
	@echo "Migrations + seed (DATABASE_URL from .env, works against any postgres):"
	@echo "  make migrate-up      apply all pending migrations"
	@echo "  make migrate-down    roll back the most recent migration"
	@echo "  make migrate-reset   drop everything and re-apply (DESTRUCTIVE)"
	@echo "  make migrate-version print current migration version"
	@echo "  make seed            load dev seed data"
	@echo ""
	@echo "Run:"
	@echo "  make backend         go run the API"
	@echo "  make frontend        npm run dev on :5173"
	@echo ""
	@echo "Tests:"
	@echo "  make test-db-create  create cafedb_test (one-time, local docker only)"
	@echo "  make test            run the integration test suite"

db-up:
	docker-compose up -d
	@echo "waiting for postgres to accept connections..."
	@until docker exec cafe-postgres pg_isready -U postgres -d cafedb >/dev/null 2>&1; do sleep 1; done
	@echo "postgres ready on localhost:5432"

db-down:
	docker-compose down

db-logs:
	docker-compose logs -f postgres

db-shell:
	docker exec -it cafe-postgres psql -U postgres -d cafedb

migrate-up:
	cd backend && go run ./cmd/migrate up

migrate-down:
	cd backend && go run ./cmd/migrate down

migrate-reset:
	cd backend && go run ./cmd/migrate reset

migrate-version:
	cd backend && go run ./cmd/migrate version

seed:
	cd backend && go run ./cmd/seed

backend:
	cd backend && go run .

frontend:
	cd frontend && npm run dev

# ----- tests -----

# Create a separate database (local docker only). For cloud DBs, create a
# `cafedb_test` schema in your provider's console and set TEST_DATABASE_URL
# manually.
test-db-create:
	docker exec cafe-postgres psql -U postgres -c "CREATE DATABASE cafedb_test" || echo "(already exists, that's fine)"

test-db-drop:
	docker exec cafe-postgres psql -U postgres -c "DROP DATABASE IF EXISTS cafedb_test"

# Run the integration test suite. The tests connect to TEST_DATABASE_URL,
# defaulting to local cafedb_test. Override with TEST_DATABASE_URL=... make test
# to run against a remote DB.
test:
	cd backend && go test ./internal/tests/... -v -count=1
