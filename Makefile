COMPOSE = docker compose
USER_SERVICE_DIR = user-service

.PHONY: help \
        infra-up infra-down up down ps \
        build-user start-user stop-user restart-user logs-user deploy-user \
        test-user test-user-v test-user-handler test-user-service test-integration-user \
        run-user \
        db-shell db-reset-user db-restart db-nuke db-seed \
        env

# ── Default ────────────────────────────────────────────────────────────────────
help:
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@echo "Infrastructure"
	@echo "  infra-up               Start postgres + redis"
	@echo "  infra-down             Stop postgres + redis"
	@echo "  up                     Start full stack"
	@echo "  down                   Stop full stack"
	@echo "  ps                     Show container status"
	@echo ""
	@echo "user-service (container)"
	@echo "  build-user             Build user-service image"
	@echo "  start-user             Start user-service container"
	@echo "  stop-user              Stop user-service container"
	@echo "  restart-user           Restart user-service container"
	@echo "  logs-user              Follow user-service logs"
	@echo "  deploy-user            Rebuild and redeploy user-service container"
	@echo ""
	@echo "user-service (tests)"
	@echo "  test-user              Run all unit tests (race detector)"
	@echo "  test-user-v            Run all unit tests (verbose + race)"
	@echo "  test-user-handler      Run handler tests only"
	@echo "  test-user-service      Run service-layer tests only"
	@echo "  test-integration-user  Run integration tests (requires infra-up)"
	@echo ""
	@echo "user-service (dev)"
	@echo "  run-user               Run user-service locally (outside Docker)"
	@echo ""
	@echo "Database"
	@echo "  db-shell               Open psql shell"
	@echo "  db-reset-user          Drop user tables + restart container"
	@echo "  db-restart             Restart postgres container"
	@echo "  db-nuke                Wipe all data volumes + reinitialise schemas"
	@echo "  db-seed                Insert sample users (admin/customer/seller)"
	@echo ""
	@echo "Setup"
	@echo "  env                    Copy .env.example to .env"
	@echo ""

# ── Infrastructure ─────────────────────────────────────────────────────────────
infra-up:
	$(COMPOSE) up -d postgres redis

infra-down:
	$(COMPOSE) stop postgres redis

up:
	$(COMPOSE) up -d

down:
	$(COMPOSE) down

ps:
	$(COMPOSE) ps

# ── user-service container ─────────────────────────────────────────────────────
build-user:
	$(COMPOSE) build user-service

start-user:
	$(COMPOSE) up -d user-service

stop-user:
	$(COMPOSE) stop user-service

restart-user:
	$(COMPOSE) restart user-service

logs-user:
	$(COMPOSE) logs -f user-service

deploy-user:
	$(COMPOSE) build user-service && $(COMPOSE) up -d user-service

# ── user-service tests ─────────────────────────────────────────────────────────
test-user:
	cd $(USER_SERVICE_DIR) && go test -race ./...

test-user-v:
	cd $(USER_SERVICE_DIR) && go test -race -v ./...

test-user-handler:
	cd $(USER_SERVICE_DIR) && go test -race ./internal/handler/...

test-user-service:
	cd $(USER_SERVICE_DIR) && go test -race ./internal/service/...

test-integration-user:
	cd $(USER_SERVICE_DIR) && go test -tags=integration -v -race ./internal/integration/

# ── user-service dev ───────────────────────────────────────────────────────────
run-user:
	cd $(USER_SERVICE_DIR) && go run ./cmd/server/main.go

# ── Database ───────────────────────────────────────────────────────────────────
db-shell:
	docker exec -it ecommerce-postgres psql -U postgres

db-restart:
	$(COMPOSE) restart postgres

db-nuke:
	$(COMPOSE) down -v
	$(COMPOSE) up -d postgres redis

db-seed:
	docker exec -i ecommerce-postgres psql -U postgres < script/sample_users.sql

db-reset-user:
	docker exec ecommerce-postgres psql -U postgres -d ecommerce_users \
	  -c "DROP TABLE IF EXISTS user_addresses, user_profiles, auth_tokens, users CASCADE;"
	$(COMPOSE) restart user-service

# ── Setup ──────────────────────────────────────────────────────────────────────
env:
	cp .env.example .env
