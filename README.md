# Pangreksa AI Gateway Engine

Enterprise-grade LLM gateway daemon system written in Go.
Implements the architecture defined in [`docs/SRS-AI-gateway-engine.md`](docs/SRS-AI-gateway-engine.md).

---

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│                         GATEWAY DAEMON (cmd/daemon)                      │
│                                                                          │
│  Proxy ports (one per provider)                                          │
│  :8080 OpenAI · :8081 Claude · :8082 Moonshot · :8083 Ollama            │
│  ┌────────────────────────────────────────────────────────────────────┐  │
│  │ Auth → Entitlement → Registry → Policy → LLM Dispatch → Response  │  │
│  └────────────────────────────────────────────────────────────────────┘  │
│                         │                      │                         │
│                Kafka produce              Redis cache                     │
│              (TransactionEvent)         (entitlement L2)                 │
└──────────────────────────────────────────────────────────────────────────┘
         ▲                   ▼
   POST /config        llm.transactions
   POST /entitlement
         │
┌──────────────────────────────────────────────────────────────────────────┐
│                      CENTRAL SERVER (cmd/server)                         │
│                                                                          │
│  Port 9000                                                               │
│  HTTP: /health /config /entitlement /admin/*                             │
│  Kafka Consumer: llm.transactions → request_logs + cost_records          │
│  PostgreSQL: full persistent store                                       │
│  Redis: pub/sub user:invalidate → daemon cache flush                     │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Binaries

| Binary | Path | Description |
|--------|------|-------------|
| Gateway Daemon | `cmd/daemon` | Hot-path LLM proxy, ports 8080–8083 |
| Central Server | `cmd/server` | Control plane, port 9000 |
| Client CLI | `cmd/client` | Interactive test client |
| Seed | `cmd/seed` | Insert dev seed data (org, user, gateway config) |
| Update Config | `cmd/update-config` | Encrypt & store real provider API keys |

---

## Quick Start

### Prerequisites

- Go 1.22+
- Access to infrastructure on `udocker01` (see [Infrastructure](#infrastructure))

### 1. Seed the database (first time only)

```powershell
.\scripts\seed-dev.ps1
```

### 2. Store real API keys (first time only)

```powershell
.\scripts\update-config.ps1
```

### 3. Start Central Server

```powershell
.\scripts\run-server.ps1
```

### 4. Start Gateway Daemon

```powershell
.\scripts\run-daemon.ps1
```

### 5. Run the test client

```powershell
# OpenAI (default)
.\scripts\run-client.ps1

# Or pick a specific proxy:
go run ./cmd/client -proxy claude   -model claude-sonnet-4-5
go run ./cmd/client -proxy openai   -model gpt-4o
go run ./cmd/client -proxy moonshot -model moonshot-v1-8k
go run ./cmd/client -proxy ollama   -model llama3

# Single-shot:
go run ./cmd/client -proxy claude -model claude-sonnet-4-5 -message "Hello"

# With streaming:
go run ./cmd/client -proxy claude -model claude-sonnet-4-5 -stream
```

### VS Code / Claude Code debug configs

`.claude/launch.json` has ready-to-use configurations: **Gateway Engine** and **Central Server Engine**, with all dev env vars pre-filled.

---

## Proxy Port Mapping

| `-proxy` flag | Port | Default model | Upstream |
|---------------|------|---------------|----------|
| `openai` | 8080 | `gpt-4o` | api.openai.com |
| `claude` | 8081 | `claude-sonnet-4-5` | api.anthropic.com |
| `moonshot` | 8082 | `moonshot-v1-8k` | api.moonshot.cn |
| `ollama` | 8083 | `llama3` | localhost:11434 |

---

## Scripts

| Script | Purpose |
|--------|---------|
| `scripts/run-server.ps1` | Set env vars + start Central Server |
| `scripts/run-daemon.ps1` | Set env vars + start Gateway Daemon |
| `scripts/run-client.ps1` | Set token + start interactive client |
| `scripts/seed-dev.ps1` | Apply migrations + insert dev org/user/config |
| `scripts/update-config.ps1` | Encrypt + store real provider API keys (**contains secrets — do not commit**) |

---

## Build

```powershell
make build-daemon    # → bin/gateway-daemon
make build-server    # → bin/central-server
make build-client    # → bin/client
make all             # Build all three
```

---

## Test & Lint

```powershell
make test   # go test ./... -race -count=1 -timeout 120s
make lint   # golangci-lint run ./...
make vet    # go vet ./...
```

---

## Database Migrations

Migrations in `migrations/` are applied automatically by the Central Server on startup.

```powershell
make migrate-up    # apply all pending
make migrate-down  # roll back last
```

---

## Environment Variables

### Gateway Daemon

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GATEWAY_ID` | Yes | — | Unique daemon instance ID |
| `CONFIG_SERVER_URL` | Yes | — | Central Server base URL |
| `DAEMON_API_KEY` | Yes | — | Auth key for Central Server calls |
| `REDIS_URL` | Yes | — | Redis connection string |
| `KAFKA_BROKERS` | Yes | — | Comma-separated Kafka broker addresses |
| `ENCRYPT_KEY` | Yes | — | 64-char hex AES-256 key |
| `LOG_LEVEL` | No | `info` | debug/info/warn/error |
| `POLL_INTERVAL_SEC` | No | `30` | Config poll interval (seconds) |
| `MAX_LIVENESS_RETRIES` | No | `0` | 0 = retry indefinitely |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | No | — | OTEL Collector gRPC endpoint |

Full reference: `configs/daemon.env.example`

### Central Server

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | PostgreSQL DSN |
| `REDIS_URL` | Yes | — | Redis connection string |
| `KAFKA_BROKERS` | Yes | — | Kafka broker addresses |
| `SERVER_PORT` | No | `9000` | HTTP listen port |
| `ADMIN_API_KEY` | Yes | — | Admin endpoint auth key |
| `DAEMON_API_KEY` | Yes | — | Daemon endpoint auth key |
| `ENCRYPT_KEY` | Yes | — | 64-char hex AES-256 key |
| `LOG_LEVEL` | No | `info` | debug/info/warn/error |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | No | — | OTEL Collector gRPC endpoint |

Full reference: `configs/server.env.example`

### Client CLI

| Flag / Env var | Default | Description |
|----------------|---------|-------------|
| `-proxy` / `DAEMON_PROXY` | `""` | Proxy name: openai \| claude \| moonshot \| ollama |
| `-model` / `DAEMON_MODEL` | proxy default | Model name |
| `-token` / `DAEMON_TOKEN` | dev UUID | Bearer token (dev user UUID) |
| `-url` / `DAEMON_URL` | `http://localhost:8080` | Direct URL (overridden by `-proxy`) |
| `-host` / `DAEMON_HOST` | `localhost` | Daemon host (used with `-proxy`) |
| `-provider` / `DAEMON_PROVIDER` | `""` | Display label only |
| `-system` / `DAEMON_SYSTEM` | `""` | System prompt |
| `-stream` | `false` | Enable SSE streaming |
| `-message` | `""` | Single-shot message |

---

## Infrastructure

| Service | Address | Notes |
|---------|---------|-------|
| PostgreSQL | `192.168.32.161:5432` | db=pangreksa, user=pangreksa, pw=devpassword |
| Redis | `192.168.32.161:6379` | Standalone, no auth in dev |
| Kafka | `192.168.32.161:9094` | External listener; topic: `llm.transactions` |
| OTEL Collector | `192.168.32.161:4317` | gRPC OTLP endpoint |
| Jaeger UI | `http://192.168.32.161:16686` | Distributed trace browser |

All containers run on `udocker01`. SSH access: `medisagi@udocker01` (passwordless key auth).

---

## Key Design Decisions

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Kafka client | `segmentio/kafka-go` | Pure Go, CGO-free, Windows-native |
| Logger | `github.com/rs/zerolog` | Structured JSON, zero-allocation hot path |
| DB driver | `github.com/jackc/pgx/v5` | Native PostgreSQL protocol |
| Migrations | `golang-migrate` | Versioned SQL, auto-apply on startup |
| Tracing | OpenTelemetry OTLP → Jaeger | Vendor-neutral, 100% request coverage |
| Encryption | AES-256-GCM | NIST-approved authenticated encryption |

---

## SRS References

- Gateway Engine: [`docs/SRS-AI-gateway-engine.md`](docs/SRS-AI-gateway-engine.md)
- Client CLI: [`docs/SRS-AI-gateway-clientCLI.md`](docs/SRS-AI-gateway-clientCLI.md)
