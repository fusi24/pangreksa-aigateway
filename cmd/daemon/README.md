# Gateway Daemon (`cmd/daemon`)

The Gateway Daemon is the hot-path LLM request processing engine.
It is a stateless, horizontally scalable Go binary that processes all LLM requests
with P95 overhead < 50ms (excluding LLM provider latency).

---

## Startup Sequence

The daemon follows a strict initialization order. Each step must succeed before the next begins:

```
1. Load & validate configuration from environment variables
   │
2. Initialize logger (zerolog, JSON to stdout)
   │
3. Initialize OTEL tracer (otlptracegrpc → OTEL Collector)
   │
4. Liveness probe loop → POST {CONFIG_SERVER_URL}/health
   │  Retries every 5s until success (or MAX_LIVENESS_RETRIES exceeded)
   │
5. Fetch initial GatewayConfig → POST {CONFIG_SERVER_URL}/config
   │  Stores config in memory; sets current version SHA-256
   │
6. Connect to Redis (entitlement L2 cache + pub/sub subscriber)
   │
7. Connect to Kafka producer (fire-and-forget TransactionEvent publisher)
   │
8. Start config poller goroutine (polls every POLL_INTERVAL_SEC seconds)
   │
9. Subscribe to Redis channel "user:invalidate" (flushes L1 entitlement cache)
   │
10. Start provider proxy listeners (one per ProxyConfig in GatewayConfig.Proxies)
    │  Each listener runs its own HTTP server goroutine
    │
11. Start health / metrics endpoint (GET /health, GET /metrics)
    │
12. Block on OS signal (SIGINT, SIGTERM) → graceful shutdown
```

---

## Hot-Path Pipeline

Every incoming LLM request flows through this pipeline synchronously:

```
CLIENT REQUEST (POST /v1/chat/completions or /v1/messages)
    │
    ▼
[1] Auth — extract Bearer token (PAT / VAT / JWT)
    │       verify token hash against Redis / Central Server
    │
    ▼
[2] Entitlement Resolve
    │       L1: sync.Map lookup (< 1ms P99)
    │       L2: Redis GET with TTL 60s (< 2ms P95)
    │       L3: POST /entitlement to Central Server (on miss)
    │
    ▼
[3] Prompt Registry
    │       If request.PromptFQN set → render template with Variables
    │       Inject rendered system prompt into Messages[0]
    │
    ▼
[4] Skill Registry
    │       If skills enabled → inject skill catalog (name+description only)
    │       into system context for LLM skill selection
    │
    ▼
[5] Input Guardrails (hook: llm_input)
    │       PII detection, injection detection, content moderation, regex
    │       Mode: block (synchronous) or validate (async parallel)
    │
    ▼
[6] Budget Check
    │       Redis INCRBYFLOAT estimated cost against first matching rule
    │       HTTP 429 if limit exceeded
    │
    ▼
[7] Rate Limit Check
    │       Redis Lua sliding window counter
    │       HTTP 429 + Retry-After header if limit exceeded
    │
    ▼
[8] LLM Dispatch
    │       POST to provider base URL with decrypted API key
    │       Streaming (SSE) or blocking response
    │       Circuit breaker: 5 failures / 10s → open
    │
    ▼
[9] Output Guardrails (hook: llm_output)
    │       PII redaction, secrets detection, content moderation
    │       Applied to response body before returning to client
    │
    ▼
[10] Response to Client
         Add response headers:
           X-Gateway-Request-ID
           X-Gateway-Latency-Ms
           X-Cache-Status
           X-Guardrails-Applied
    │
    ▼ (fire-and-forget, non-blocking)
[11] Publish TransactionEvent → Kafka topic llm.transactions
```

---

## Provider Proxy Ports

| Port | Provider | Protocol |
|------|----------|----------|
| 8080 | OpenAI | POST /v1/chat/completions |
| 8081 | Anthropic Claude | POST /v1/messages |
| 8082 | Moonshot / Kimi | POST /v1/chat/completions |
| 8083 | Ollama | POST /api/chat |
| 9090 | Health / Metrics | GET /health, GET /metrics |

Ports are configurable via `PROXY_PORT_*` environment variables.

---

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `GATEWAY_ID` | Yes | — | Unique daemon instance identifier. Appears in all logs and TransactionEvents. |
| `CONFIG_SERVER_URL` | Yes | — | Base URL of Central Server (e.g. `http://localhost:9000`). |
| `DAEMON_API_KEY` | Yes | — | Bearer token for Central Server API calls. |
| `REDIS_URL` | Yes | — | Redis connection string (`redis://<host>:<port>`). |
| `KAFKA_BROKERS` | Yes | — | Comma-separated Kafka broker addresses. |
| `KAFKA_TOPIC_TRANSACTIONS` | No | `llm.transactions` | Kafka topic for TransactionEvents. |
| `ENCRYPT_KEY` | Yes | — | 64-char hex AES-256 key for decrypting `api_key_enc` values. |
| `LOG_LEVEL` | No | `info` | `trace`/`debug`/`info`/`warn`/`error` |
| `POLL_INTERVAL_SEC` | No | `30` | Config poll interval in seconds. |
| `MAX_LIVENESS_RETRIES` | No | `0` | 0 = retry indefinitely until Central Server responds. |
| `PROXY_PORT_OPENAI` | No | `8080` | Listen port for OpenAI proxy. |
| `PROXY_PORT_CLAUDE` | No | `8081` | Listen port for Anthropic proxy. |
| `PROXY_PORT_MOONSHOT` | No | `8082` | Listen port for Moonshot proxy. |
| `PROXY_PORT_OLLAMA` | No | `8083` | Listen port for Ollama proxy. |
| `HEALTH_PORT` | No | `9090` | Listen port for health/metrics endpoint. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | No | — | OTEL Collector gRPC endpoint (e.g. `192.168.32.161:4317`). |
| `OTEL_SERVICE_NAME` | No | `gateway-daemon` | Service name in traces. |
| `OTEL_INSECURE` | No | `true` | Disable TLS on OTLP connection (dev only). |

Full examples with comments: `configs/daemon.env.example`
