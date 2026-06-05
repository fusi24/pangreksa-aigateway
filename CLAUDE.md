# Pangreksa AI Gateway Engine

## What This Is
Enterprise-grade LLM gateway daemon system. Go binaries:
- **Gateway Daemon** (`cmd/daemon`) — hot-path proxy: entitlement, registry, policy, LLM dispatch
- **Central Server** (`cmd/server`) — control plane: config, entitlement API, Kafka consumer, PostgreSQL
- **Client CLI** (`cmd/client`) — interactive test client with `-proxy` / `-model` flags
- **Seed** (`cmd/seed`) — inserts dev org, user, entitlement, and initial gateway config
- **Update Config** (`cmd/update-config`) — encrypts real provider API keys and stores in DB

## Module
`github.com/pangreksa/ai-gateway-engine` (Go 1.22)

## Infrastructure (udocker01 @ 192.168.32.161)
- PostgreSQL: `192.168.32.161:5432` (pangreksa/devpassword)
- Redis: `192.168.32.161:6379`
- Kafka: `192.168.32.161:9094` (external listener)
- OTEL: `192.168.32.161:4317`
- Jaeger: `http://192.168.32.161:16686`

SSH: `medisagi@udocker01` (passwordless key auth)

## Proxy Port Mapping
| Provider | Port | Default model |
|----------|------|---------------|
| openai | 8080 | gpt-4o |
| claude | 8081 | claude-sonnet-4-5 |
| moonshot | 8082 | moonshot-v1-8k |
| ollama | 8083 | llama3 |

## Run Locally (PowerShell)
```powershell
# First time only:
.\scripts\seed-dev.ps1        # apply migrations + insert dev data
.\scripts\update-config.ps1   # store encrypted real API keys

# Every time:
.\scripts\run-server.ps1      # Terminal 1 — Central Server on :9000
.\scripts\run-daemon.ps1      # Terminal 2 — Gateway Daemon on :8080–8083

# Test client:
.\scripts\run-client.ps1
go run ./cmd/client -proxy claude -model claude-sonnet-4-5
go run ./cmd/client -proxy openai -message "hello"
```

## Key Design Decisions
- **Kafka client**: `segmentio/kafka-go` (pure Go, Windows-native; CGO-free)
- **Logging**: `github.com/rs/zerolog` (structured JSON)
- **DB driver**: `github.com/jackc/pgx/v5`
- **Migrations**: `golang-migrate` (auto-applied on server startup)
- **Tracing**: OpenTelemetry OTLP → OTEL Collector → Jaeger
- **Client token**: Bearer token = dev user UUID `00000000-0000-0000-0000-000000000002`
- **API key encryption**: AES-256-GCM, stored as base64; dev ENCRYPT_KEY is 64 hex zeros
- **`scripts/update-config.ps1`** contains plaintext API keys — never commit this file

## Testing
```bash
go test ./... -race -count=1
```

## Lint
```bash
golangci-lint run ./...
```

## SRS References
- `docs/SRS-AI-gateway-engine.md`
- `docs/SRS-AI-gateway-clientCLI.md`
