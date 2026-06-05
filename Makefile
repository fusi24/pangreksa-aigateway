# ─────────────────────────────────────────────────────────────────────────────
# Pangreksa AI Gateway Engine — Makefile
# ─────────────────────────────────────────────────────────────────────────────
# Usage:
#   make build-daemon   # Compile gateway-daemon binary
#   make build-server   # Compile central-server binary
#   make run-daemon     # Run gateway-daemon from source (dev)
#   make run-server     # Run central-server from source (dev)
#   make test           # Run all tests with race detector
#   make lint           # Run golangci-lint
#   make fmt            # Format all Go source files
#   make vet            # Run go vet
#   make tidy           # Tidy go.mod / go.sum
#   make clean          # Remove compiled binaries
#   make migrate-up     # Apply pending SQL migrations
#   make migrate-down   # Roll back the last SQL migration
# ─────────────────────────────────────────────────────────────────────────────

# Tool names — override if your PATH differs.
GO            := go
GOLANGCI_LINT := golangci-lint
MIGRATE       := migrate

# Output directory for compiled binaries.
BIN_DIR := bin

# Migration settings — sourced from the server env example.
MIGRATIONS_DIR := migrations
DATABASE_URL   ?= postgres://pangreksa:devpassword@192.168.32.161:5432/pangreksa?sslmode=disable

# Go build flags — strip debug info for smaller binaries.
LDFLAGS := -ldflags="-s -w"

# Daemon environment variables for local development.
# Override any of these on the command line: make run-daemon LOG_LEVEL=trace
GATEWAY_ID                    ?= gw-dev-001
CONFIG_SERVER_URL             ?= http://localhost:9000
DAEMON_API_KEY                ?= dev-daemon-key-changeme
REDIS_URL                     ?= redis://192.168.32.161:6379
KAFKA_BROKERS                 ?= 192.168.32.161:9094
POLL_INTERVAL_SEC             ?= 30
MAX_LIVENESS_RETRIES          ?= 0
ENCRYPT_KEY                   ?= 0000000000000000000000000000000000000000000000000000000000000000
LOG_LEVEL                     ?= debug
OTEL_EXPORTER_OTLP_ENDPOINT   ?= 192.168.32.161:4317

# Server environment variables for local development.
SERVER_PORT     ?= 9000
ADMIN_API_KEY   ?= dev-admin-key-changeme

.PHONY: all build-daemon build-server build-client run-daemon run-server run-client \
        test lint fmt vet tidy clean migrate-up migrate-down seed

## all: Build all binaries (default target).
all: build-daemon build-server build-client

## build-daemon: Compile the Gateway Daemon binary to bin/gateway-daemon.
build-daemon:
	@echo "==> Building gateway-daemon..."
	@mkdir -p $(BIN_DIR)
	$(GO) build $(LDFLAGS) -o $(BIN_DIR)/gateway-daemon ./cmd/daemon

## build-server: Compile the Central Server binary to bin/central-server.
build-server:
	@echo "==> Building central-server..."
	@mkdir -p $(BIN_DIR)
	$(GO) build $(LDFLAGS) -o $(BIN_DIR)/central-server ./cmd/server

## build-client: Compile the interactive test client binary to bin/client.
build-client:
	@echo "==> Building client..."
	@mkdir -p $(BIN_DIR)
	$(GO) build $(LDFLAGS) -o $(BIN_DIR)/client ./cmd/client

## run-client: Run the interactive test client from source (go run) with dev defaults.
run-client:
	@echo "==> Running client (interactive)..."
	DAEMON_URL=http://localhost:8080 \
	DAEMON_MODEL=gpt-4o \
	DAEMON_TOKEN=dev-test-pat-token-hash \
	$(GO) run ./cmd/client

## run-daemon: Run the Gateway Daemon from source (go run) with dev env vars.
run-daemon:
	@echo "==> Running gateway-daemon (dev)..."
	GATEWAY_ID=$(GATEWAY_ID) \
	CONFIG_SERVER_URL=$(CONFIG_SERVER_URL) \
	DAEMON_API_KEY=$(DAEMON_API_KEY) \
	REDIS_URL=$(REDIS_URL) \
	KAFKA_BROKERS=$(KAFKA_BROKERS) \
	POLL_INTERVAL_SEC=$(POLL_INTERVAL_SEC) \
	MAX_LIVENESS_RETRIES=$(MAX_LIVENESS_RETRIES) \
	ENCRYPT_KEY=$(ENCRYPT_KEY) \
	LOG_LEVEL=$(LOG_LEVEL) \
	OTEL_EXPORTER_OTLP_ENDPOINT=$(OTEL_EXPORTER_OTLP_ENDPOINT) \
	$(GO) run ./cmd/daemon

## run-server: Run the Central Server from source (go run) with dev env vars.
run-server:
	@echo "==> Running central-server (dev)..."
	DATABASE_URL=$(DATABASE_URL) \
	REDIS_URL=$(REDIS_URL) \
	KAFKA_BROKERS=$(KAFKA_BROKERS) \
	SERVER_PORT=$(SERVER_PORT) \
	ADMIN_API_KEY=$(ADMIN_API_KEY) \
	DAEMON_API_KEY=$(DAEMON_API_KEY) \
	ENCRYPT_KEY=$(ENCRYPT_KEY) \
	LOG_LEVEL=$(LOG_LEVEL) \
	OTEL_EXPORTER_OTLP_ENDPOINT=$(OTEL_EXPORTER_OTLP_ENDPOINT) \
	$(GO) run ./cmd/server

## test: Run all unit and integration tests with race detector.
test:
	@echo "==> Running tests..."
	$(GO) test ./... -race -count=1 -timeout 120s

## lint: Run golangci-lint across all packages.
lint:
	@echo "==> Running golangci-lint..."
	$(GOLANGCI_LINT) run ./...

## fmt: Format all Go source files with gofmt.
fmt:
	@echo "==> Formatting Go source..."
	$(GO) fmt ./...

## vet: Run go vet static analysis.
vet:
	@echo "==> Running go vet..."
	$(GO) vet ./...

## tidy: Tidy go.mod and go.sum, removing unused dependencies.
tidy:
	@echo "==> Tidying go.mod..."
	$(GO) mod tidy

## clean: Remove compiled binaries from bin/.
clean:
	@echo "==> Cleaning bin/..."
	@rm -rf $(BIN_DIR)

## migrate-up: Apply all pending up-migrations using golang-migrate CLI.
## Requires: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
migrate-up:
	@echo "==> Applying migrations (up)..."
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up

## migrate-down: Roll back the last applied migration.
migrate-down:
	@echo "==> Rolling back last migration (down)..."
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down 1

## seed: Insert development seed data (idempotent — safe to run multiple times).
seed:
	DATABASE_URL=postgres://pangreksa:devpassword@192.168.32.161:5432/pangreksa?sslmode=disable \
	$(GO) run ./cmd/seed
