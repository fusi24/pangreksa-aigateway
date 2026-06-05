# Central Server (`cmd/server`)

The Central Server is the control plane for the AI Gateway Engine.
It serves configuration and entitlement data to Gateway Daemons, persists all
transaction data via a Kafka consumer, and provides admin management endpoints.

---

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/health` | None | Liveness probe. Returns `{"status":"ok"}`. |
| `POST` | `/config` | `DAEMON_API_KEY` | Returns full `GatewayConfig` JSON, or HTTP 304 if version unchanged. |
| `POST` | `/entitlement` | `DAEMON_API_KEY` | Returns `UserEntitlement` for a given `user_id` + `org_id`. |
| `POST` | `/admin/config` | `ADMIN_API_KEY` | Create or update the `GatewayConfig` for a gateway. |
| `POST` | `/admin/entitlement` | `ADMIN_API_KEY` | Update entitlement for a user and invalidate their cache. |
| `POST` | `/admin/invalidate` | `ADMIN_API_KEY` | Publish `user:invalidate` to Redis pub/sub to flush daemon L1 cache. |

### Request / Response Details

```
POST /config
  Authorization: Bearer {DAEMON_API_KEY}
  Body: { "gateway_id": "gw-001", "current_version": "sha256hex" }

  200 → GatewayConfig JSON (full config, new version available)
  304 → (empty body, config version unchanged)
  401 → { "error": "unauthorized" }

POST /entitlement
  Authorization: Bearer {DAEMON_API_KEY}
  Body: { "user_id": "uuid", "org_id": "uuid" }

  200 → UserEntitlement JSON
  404 → { "error": "user_not_found" }
  401 → { "error": "unauthorized" }

POST /admin/config
  Authorization: Bearer {ADMIN_API_KEY}
  Body: partial or full GatewayConfig

  200 → { "version": "new_sha256", "applied_at": "ISO8601" }
  400 → { "error": "validation_failed", "details": [...] }

POST /admin/entitlement
  Authorization: Bearer {ADMIN_API_KEY}
  Body: { "user_id": "uuid", "changes": { "allowed_skills": [...] } }

  200 → { "updated": true, "invalidated": true }
  404 → { "error": "user_not_found" }

POST /admin/invalidate
  Authorization: Bearer {ADMIN_API_KEY}
  Body: { "user_id": "uuid" }

  200 → { "published": true, "channel": "user:invalidate" }
```

---

## Kafka Consumer

| Setting | Value |
|---------|-------|
| Topic | `llm.transactions` |
| Consumer Group | `central-server-consumer` |
| Delivery | At-least-once |
| Offset commit | After successful DB INSERT (not before) |
| Idempotency | `event_id` used as unique key (`ON CONFLICT DO NOTHING`) |

On each `TransactionEvent` message:

1. Validate JSON schema
2. Enrich with pricing lookup (cost per token for provider+model)
3. `INSERT INTO request_logs` (idempotent via `event_id`)
4. `INSERT INTO cost_records`
5. `INSERT INTO audit_log`
6. Commit Kafka offset

---

## Database Migrations

Migrations are applied automatically on startup when `AUTO_MIGRATE=true` (default).

Migration files are in the `migrations/` directory at the project root.
The Central Server Docker image includes the migrations directory at `/migrations`.

To run migrations manually:

```bash
# Install golang-migrate CLI (one-time)
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Apply all pending
migrate -path migrations -database "$DATABASE_URL" up

# Roll back last migration
migrate -path migrations -database "$DATABASE_URL" down 1
```

---

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | PostgreSQL DSN. Example: `postgres://pangreksa:pw@host:5432/pangreksa?sslmode=disable` |
| `REDIS_URL` | Yes | — | Redis connection string. |
| `KAFKA_BROKERS` | Yes | — | Comma-separated Kafka broker addresses. |
| `SERVER_PORT` | No | `9000` | HTTP API listen port. |
| `SERVER_READ_TIMEOUT_SEC` | No | `30` | HTTP read timeout in seconds. |
| `SERVER_WRITE_TIMEOUT_SEC` | No | `30` | HTTP write timeout in seconds. |
| `ADMIN_API_KEY` | Yes | — | Bearer token for `/admin/*` endpoints. |
| `DAEMON_API_KEY` | Yes | — | Bearer token for `/config` and `/entitlement` endpoints. |
| `ENCRYPT_KEY` | Yes | — | 64-char hex AES-256 key for encrypting/decrypting provider API keys. |
| `MIGRATIONS_PATH` | No | `migrations` | Path to SQL migration files. |
| `AUTO_MIGRATE` | No | `true` | Apply pending migrations on startup. |
| `LOG_LEVEL` | No | `info` | `trace`/`debug`/`info`/`warn`/`error` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | No | — | OTEL Collector gRPC endpoint. |
| `OTEL_SERVICE_NAME` | No | `central-server` | Service name in traces. |
| `OTEL_INSECURE` | No | `true` | Disable TLS on OTLP gRPC (dev only). |
| `ENTITLEMENT_CACHE_TTL_SEC` | No | `60` | Redis L2 cache TTL for `UserEntitlement`. |
| `KAFKA_CONSUMER_GROUP` | No | `central-server-consumer` | Kafka consumer group ID. |
| `KAFKA_COMMIT_BATCH_SIZE` | No | `100` | Messages per Kafka offset commit batch. |
| `KAFKA_MAX_WAIT_MS` | No | `500` | Max wait for batch fill before processing. |

Full examples with comments: `configs/server.env.example`
