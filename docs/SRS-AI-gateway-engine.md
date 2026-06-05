# Software Requirements Specification
# Pangreksa AI Gateway Engine

---

| Field         | Value                                          |
|---------------|------------------------------------------------|
| Document ID   | SRS-AI-GATEWAY-ENGINE-001                      |
| Version       | 1.0                                            |
| Status        | Draft                                          |
| Prepared by   | AI Software Architect (iSAQB CPSA-A Aligned)  |
| Date          | 2026-06-04                                     |
| Standard      | iSAQB CPSA-A / ISO/IEC 29148:2018             |
| Technology    | Go (latest stable), Docker, PostgreSQL, Redis, Kafka |

---

## Table of Contents

1. Introduction
2. System Overview
3. Architecture Overview
4. Daemon Requirements
5. Central Server Requirements
6. Message Contracts
7. Platform Infrastructure
8. Non-Functional Requirements
9. API Contracts
10. Data Architecture & ERD
11. Security Architecture
12. Deployment Architecture
13. Risks & Mitigation
14. Appendix

---

## 1. Introduction

### 1.1 Purpose

This Software Requirements Specification (SRS) defines the complete functional and
non-functional requirements for the **Pangreksa AI Gateway Engine** — a high-performance,
enterprise-grade LLM gateway daemon system implemented in Go. This document targets:
software architects, Go developers, DevOps engineers, QA engineers, and product owners.

The system is designed as a two-component architecture:
- **Gateway Daemon** — the hot-path LLM request processor
- **Central Server** — the control plane, configuration authority, and persistence layer

### 1.2 Scope

**System Name:** Pangreksa AI Gateway Engine

**In Scope:**
- Gateway Daemon written in Go: liveness probe, config poller, entitlement resolver,
  proxy, registry (prompt/skill/MCP), policy (guardrails/budget/rate limit), LLM dispatch
- Central Server written in Go: config store, entitlement management, Kafka consumer,
  PostgreSQL repository, Redis pub/sub publisher
- Message contracts between Daemon and Central Server via HTTP POST and Kafka
- Platform infrastructure: Redis, Kafka, PostgreSQL
- Docker image delivery; Windows-compatible local testing
- Per-user entitlement: allowed prompts, skills, MCPs resolved at runtime

**Out of Scope:**
- Console/Admin UI (separate system per SRS-PANGREKSA-AIROUTERGATEWAY-001)
- LLM model training or fine-tuning
- Native mobile applications
- Billing and invoicing UI

### 1.3 Definitions & Acronyms

| Term           | Definition                                                              |
|----------------|-------------------------------------------------------------------------|
| SRS            | Software Requirements Specification                                     |
| Daemon         | Gateway Daemon — the hot-path LLM request processing engine             |
| Central Server | Control plane Go service: config, entitlement, persistence              |
| PAT            | Personal Access Token — user-bound API key                              |
| VAT            | Virtual Account Token — service-identity API key                        |
| FQN            | Fully Qualified Name — prompt version identifier                        |
| MCP            | Model Context Protocol — tool/data integration protocol                 |
| HITL           | Human-in-the-Loop — approval workflow for destructive tool calls        |
| TTL            | Time To Live — cache expiry duration                                    |
| chan Config    | Go channel carrying Config struct for inter-goroutine communication     |
| sync.Map       | Go built-in concurrent map for in-process entitlement cache             |
| Goroutine      | Go lightweight thread                                                   |
| Redis Lua      | Atomic server-side Lua scripts executed in Redis                        |
| SKILL.md       | Markdown document defining a reusable agent skill/capability            |
| Virtual Model  | Logical LLM abstraction routing to one or more real provider models     |
| RBAC           | Role-Based Access Control                                               |
| OTEL           | OpenTelemetry                                                           |

### 1.4 References

| #    | Reference                                                               |
|------|-------------------------------------------------------------------------|
| R-01 | SRS-PANGREKSA-AIROUTERGATEWAY-001 v1.0 (parent SRS)                    |
| R-02 | iSAQB CPSA-A Curriculum — https://www.isaqb.org                        |
| R-03 | ISO/IEC 29148:2018 — Requirements Engineering                           |
| R-04 | Go 1.22+ Documentation — https://go.dev/doc                            |
| R-05 | Model Context Protocol — https://modelcontextprotocol.io               |
| R-06 | OpenTelemetry Go SDK — https://opentelemetry.io/docs/languages/go      |
| R-07 | Confluent Kafka Go — https://github.com/confluentinc/confluent-kafka-go |
| R-08 | pgx PostgreSQL driver — https://github.com/jackc/pgx                   |
| R-09 | go-redis — https://github.com/redis/go-redis                           |

---

## 2. System Overview

### 2.1 System Context

```
╔══════════════════════════════════════════════════════════════════════════╗
║                    SYSTEM BOUNDARY                                       ║
║                                                                          ║
║   ┌─────────────────┐          ┌──────────────────────────────────┐      ║
║   │  CLIENT APPS    │          │        GATEWAY DAEMON            │      ║
║   │                 │  Bearer  │        (Go, Docker)              │      ║
║   │  SDKs / IDEs    │─────────►│                                  │      ║
║   │  Agent Fwks     │          │  Proxy → Registry → Policy       │      ║
║   │  n8n / Dify     │◄─────────│       → LLM dispatch             │      ║
║   └─────────────────┘  resp    └───────────────┬──────────────────┘      ║
║                                                │                         ║
║                              POST /poll        │  Kafka (txn events)     ║
║                              POST /health      │                         ║
║                                                ▼                         ║
║                                ┌──────────────────────────────────┐      ║
║                                │       CENTRAL SERVER             │      ║
║                                │       (Go, Docker)               │      ║
║                                │                                  │      ║
║                                │  Config store · Entitlement      │      ║
║                                │  Kafka consumer · Repository     │      ║
║                                └────────┬────────────┬────────────┘      ║
║                                         │            │                   ║
║                          ┌──────────────┘            └──────────┐        ║
║                          ▼                                       ▼        ║
║                  ┌───────────────┐                   ┌─────────────────┐ ║
║                  │  PostgreSQL   │                   │  Redis Cluster  │ ║
║                  │  (primary DB) │                   │  (cache+pubsub) │ ║
║                  └───────────────┘                   └─────────────────┘ ║
║                                                                          ║
║                  ┌───────────────┐                                       ║
║                  │     Kafka     │                                       ║
║                  │  (event bus)  │                                       ║
║                  └───────────────┘                                       ║
╚══════════════════════════════════════════════════════════════════════════╝

EXTERNAL:
  ┌──────────────────────────────────────────────────────────────────┐
  │  LLM Providers          MCP Servers         Identity Providers   │
  │  OpenAI · Anthropic     Slack · GitHub      Local · Entra · AD  │
  │  Moonshot · Ollama      Salesforce · 100+                        │
  └──────────────────────────────────────────────────────────────────┘
```

### 2.2 System Goals

| ID       | Goal                                                                          |
|----------|-------------------------------------------------------------------------------|
| GOAL-001 | Process LLM inquiries through a unified proxy daemon with sub-50ms overhead   |
| GOAL-002 | Enforce per-user entitlements (allowed prompts, skills, MCPs) at runtime      |
| GOAL-003 | Apply registry (prompt/skill/MCP) and policy (guardrail/budget/rate) per request |
| GOAL-004 | Decouple transaction persistence from hot path via Kafka                      |
| GOAL-005 | Enable config hot-reload via background poller without daemon restart         |
| GOAL-006 | Support immediate entitlement revocation via Redis pub/sub invalidation       |
| GOAL-007 | Deliver as Docker image; testable on Windows without Docker                   |

### 2.3 Constraints

| Type       | Constraint                                                              |
|------------|-------------------------------------------------------------------------|
| Language   | Go (latest stable — 1.22+) for both Daemon and Central Server          |
| Database   | PostgreSQL 16                                                           |
| Cache      | Redis 7+ (Cluster mode in production)                                   |
| Broker     | Apache Kafka (confluent-kafka-go)                                       |
| Container  | Docker; final deliverable as Docker image                               |
| Windows    | Must run natively on Windows for local testing (no Docker required)     |
| API Compat | Daemon proxy must be OpenAI API-compatible                              |
| Stateless  | Daemon pods must be stateless; all shared state in Redis/PostgreSQL     |
| Security   | TLS 1.3 in transit; AES-256 at rest; no hardcoded secrets              |

---

## 3. Architecture Overview

### 3.1 Architectural Style

**Event-Driven + Gateway Pattern + CQRS**

- **Gateway Pattern**: Daemon is the unified LLM proxy; all client traffic flows through it
- **Event-Driven**: Completed transactions published to Kafka; Central Server consumes async
- **CQRS**: Write path (Kafka -> PostgreSQL) separated from read path (config/entitlement API)
- **Pull-Based Config**: Daemon polls Central Server; Central Server never pushes to Daemon
- **Two-Layer Cache**: In-process sync.Map (nanoseconds) -> Redis (sub-millisecond) for entitlements

### 3.2 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        GATEWAY DAEMON                                   │
│                                                                         │
│  START                                                                  │
│   │                                                                     │
│   ├─[1]─► LIVENESS PROBE ─────────────────────────────────────────┐    │
│   │        POST /health to Central Server                          │    │
│   │        Retry with exponential backoff until 200 OK            │    │
│   │        BLOCKS — daemon does not proceed until OK              │    │
│   │                                                                │    │
│   ├─[2]─► CONFIG POLLER (background goroutine) ◄──────────────────┘    │
│   │        POST /config to Central Server every N seconds              │
│   │        Diffs new config vs cached config                           │
│   │        Sends update via chan Config to main goroutine              │
│   │                                                                     │
│   ├─[3]─► INVALIDATION SUBSCRIBER (background goroutine)               │
│   │        SUBSCRIBE Redis channel user:invalidate                     │
│   │        On message: DEL user:entitlement from sync.Map + Redis      │
│   │                                                                     │
│   └─[4]─► MAIN GOROUTINE — select {}                                   │
│                                                                         │
│   HOT PATH PER INQUIRY:                                                 │
│                                                                         │
│   [PROXY] → [ENTITLEMENT] → [REGISTRY] → [POLICY] → [LLM] → [KAFKA]   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 3.3 Full Hot Path Detail

```
User Request
    │
    ▼
┌──────────────────────────────────────────────────────────────────────┐
│ PROXY                                                                │
│ OpenAI :8080 · Claude :8081 · Moonshot :8082 · Ollama :8083         │
│ Normalize to unified InquiryRequest struct                           │
└──────────────────────────────┬───────────────────────────────────────┘
                               │
                               ▼
┌──────────────────────────────────────────────────────────────────────┐
│ ENTITLEMENT RESOLVER                                                 │
│                                                                      │
│  sync.Map → Redis → Central Server                                   │
│  Resolves per-user:                                                  │
│    allowed_prompts · allowed_skills · allowed_mcps                   │
│    RBAC permissions · budget_limit · rate_limit                      │
└──────────────────────────────┬───────────────────────────────────────┘
                               │
                               ▼
┌──────────────────────────────────────────────────────────────────────┐
│ REGISTRY  (from local config cache — zero network)                   │
│                                                                      │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐   │
│  │ Prompt Registry  │  │ Skill Registry   │  │ MCP Registry     │   │
│  │ FQN resolve      │  │ Progressive      │  │ Tool whitelist   │   │
│  │ Template vars    │  │ disclosure       │  │ preload/dynamic  │   │
│  │ Prompt guardrails│  │ SKILL.md inject  │  │ Cedar/OPA        │   │
│  │ Version/rollback │  │ on select        │  │ HITL flag        │   │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘   │
└──────────────────────────────┬───────────────────────────────────────┘
                               │
                               ▼
┌──────────────────────────────────────────────────────────────────────┐
│ POLICY                                                               │
│                                                                      │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐   │
│  │ Guardrails       │  │ Budget           │  │ Rate Limit       │   │
│  │ 4 hooks:         │  │ YAML ordered     │  │ Sliding window   │   │
│  │  llm_input       │  │ First match      │  │ 5s buckets       │   │
│  │  llm_output      │  │ blocks           │  │ 12 lookback      │   │
│  │  mcp_pre         │  │ All match        │  │ Redis Lua atomic │   │
│  │  mcp_post        │  │ tracked          │  │ HTTP 429         │   │
│  │ Validate/Mutate/ │  │ Redis atomic     │  │ Retry-After hdr  │   │
│  │ Block            │  │ INCRBYFLOAT      │  │                  │   │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘   │
└──────────────────────────────┬───────────────────────────────────────┘
                               │
                               ▼
┌──────────────────────────────────────────────────────────────────────┐
│ LLM PROVIDER DISPATCH                                                │
│ POST to: OpenAI / Anthropic / Moonshot / Ollama / ...               │
│ Circuit breaker per provider (5 failures in 10s → open)             │
└──────────────────────────────┬───────────────────────────────────────┘
                               │
                        ┌──────┴──────┐
                        │             │
                        ▼             ▼
               Response to      Kafka Producer
               User             fire-and-forget
                                topic: llm.transactions
```

### 3.4 Architecture Decision Records (ADR)

#### ADR-001: Go for Both Daemon and Central Server
- **Status:** Accepted
- **Context:** Need high-throughput, low-latency LLM proxy with concurrent goroutines
- **Decision:** Go 1.22+ for both services
- **Rationale:** Native goroutines map perfectly to concurrent config poller + main handler;
  single binary deployment; minimal memory footprint; ~500KB RAM for 1k user entitlements
- **Consequences:** Team must be proficient in Go

#### ADR-002: Pull-Based Config (No Push from Central Server)
- **Status:** Accepted
- **Context:** Gateway daemon can only POST; server cannot initiate connections to daemon
- **Decision:** Daemon polls Central Server via POST every N seconds
- **Rationale:** Firewall-friendly; daemon controls its own config lifecycle; stateless server
- **Consequences:** Max config propagation delay = poll interval (N seconds)

#### ADR-003: Two-Layer Entitlement Cache (sync.Map + Redis)
- **Status:** Accepted
- **Context:** Per-user entitlement must be resolved on every inquiry without RTT overhead
- **Decision:** Layer 1: sync.Map (~50ns); Layer 2: Redis (~0.5ms); Layer 3: server (~20ms)
- **Rationale:** Under 1k concurrent users, all entitlements fit in ~500KB RAM; 99%+ of
  requests served from sync.Map with zero network overhead
- **Consequences:** Entitlement changes propagated via Redis pub/sub (near-instant eviction)

#### ADR-004: Kafka for Transaction Decoupling
- **Status:** Accepted
- **Context:** DB writes must not block the LLM response path
- **Decision:** Kafka fire-and-forget after LLM response; Central Server consumes async
- **Rationale:** Decouples hot path from persistence; absorbs burst traffic; at-least-once delivery
- **Consequences:** Eventual consistency on transaction logs; idempotent consumer required

#### ADR-005: Windows-Compatible Local Testing
- **Status:** Accepted
- **Context:** Developers use Windows machines; Docker not always available locally
- **Decision:** All services use env var config; no OS-specific code paths
- **Rationale:** go build and go test must work on windows/amd64 without Docker
- **Consequences:** No Unix socket usage; file paths via filepath.Join; env vars only

---

## 4. Daemon Requirements

### 4.1 Startup Sequence

```
DAEMON STARTUP SEQUENCE
════════════════════════

main()
  │
  ├──[STEP 1] Load config from env vars
  │           CONFIG_SERVER_URL  REDIS_URL  KAFKA_BROKERS
  │           POLL_INTERVAL_SEC (default: 30)
  │           GATEWAY_ID (unique per instance)
  │
  ├──[STEP 2] Run Liveness Probe (BLOCKING)
  │
  │   ┌─────────────────────────────────────────────────────┐
  │   │  attempt = 0; backoff = 2s                          │
  │   │  loop:                                              │
  │   │    attempt++                                        │
  │   │    POST {CONFIG_SERVER_URL}/health                  │
  │   │    if 200 OK → break (proceed to Step 3)           │
  │   │    log "attempt N failed, retry in Xs"             │
  │   │    sleep(backoff)                                   │
  │   │    backoff = min(backoff * 2, 30s)                  │
  │   │    if MAX_RETRIES > 0 && attempt >= MAX_RETRIES:    │
  │   │        log.Fatal("max retries exceeded")            │
  │   │        os.Exit(1)                                   │
  │   └─────────────────────────────────────────────────────┘
  │
  ├──[STEP 3] Start Config Poller (background goroutine)
  │           → immediate first fetch on start
  │           → then every POLL_INTERVAL_SEC
  │           → on version change: send GatewayConfig to configChan
  │           → on error: log warning, keep cached config
  │
  ├──[STEP 4] Start Invalidation Subscriber (background goroutine)
  │           → SUBSCRIBE Redis "user:invalidate"
  │           → on message {user_id}:
  │               sync.Map.Delete(user_id)
  │               redis.Del("user:entitlement:" + user_id)
  │
  └──[STEP 5] Start Main Goroutine — ready to accept inquiries
              select {
                case newCfg := <-configChan:
                    reload proxy/registry/policy (hot-reload)
                case inq := <-inquiryChan:
                    process through pipeline
                case <-shutdownChan:
                    graceful shutdown
              }
```

**SRS-FR-D-001: Liveness Probe**

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| Priority           | Critical                                                             |
| Component          | internal/probe/                                                      |
| Description        | On startup, daemon MUST block until Central Server responds 200 OK  |
| Retry strategy     | Exponential backoff: initial 2s, max 30s, infinite (or MAX_RETRIES) |
| Acceptance         | Server down → daemon retries; server recovers → daemon proceeds      |

**SRS-FR-D-002: Config Poller**

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| Priority           | Critical                                                             |
| Component          | internal/poller/                                                     |
| Description        | Background goroutine POSTs /config every POLL_INTERVAL_SEC          |
| Change detection   | reflect.DeepEqual on version field; send to chan Config if changed   |
| Error handling     | Log warning; continue with last known config                         |
| Acceptance         | Config changes applied within one poll cycle; no daemon restart      |

**SRS-FR-D-003: Config Interrupt**

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| Priority           | Critical                                                             |
| Description        | Main goroutine select{} listens on configChan                       |
| Behavior           | On new Config: reload proxy ports, registry, and policy rules        |
| Constraint         | In-flight requests use old config; new rules apply to next inquiry   |

**SRS-FR-D-004: Invalidation Subscriber**

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| Priority           | Critical                                                             |
| Component          | internal/cache/invalidation/                                         |
| Description        | Background goroutine subscribes to Redis "user:invalidate" channel  |
| On message         | DEL from sync.Map AND Redis key "user:entitlement:{user_id}"        |
| Acceptance         | Admin revokes skill → daemon evicts within 100ms                     |

### 4.2 Proxy

**SRS-FR-D-005: Multi-Provider Proxy**

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| Priority           | Critical                                                             |
| Component          | internal/proxy/                                                      |
| Description        | One HTTP listener per provider, per port, as defined in Config      |
| Supported          | OpenAI, Anthropic (Messages API), Moonshot, Ollama                  |
| Acceptance         | OpenAI SDK pointed at :8080 works unchanged                          |

```
CONFIG.PROXIES EXAMPLE
══════════════════════

  provider: openai    port: 8080   base_url: https://api.openai.com
  provider: claude    port: 8081   base_url: https://api.anthropic.com
  provider: moonshot  port: 8082   base_url: https://api.moonshot.cn
  provider: ollama    port: 8083   base_url: http://ollama:11434
```

**SRS-FR-D-006: Request Normalization**

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| Priority           | Critical                                                             |
| Description        | Each adapter normalizes provider format to unified InquiryRequest   |

```go
// Unified internal request struct
type InquiryRequest struct {
    RequestID   string
    UserID      string
    OrgID       string
    Provider    string            // openai | claude | moonshot | ollama
    Model       string
    Messages    []Message
    Tools       []Tool
    Stream      bool
    Temperature float64
    MaxTokens   int
    Metadata    map[string]string
    ReceivedAt  time.Time
}
```

### 4.3 Entitlement Resolver

**SRS-FR-D-007: Two-Layer Entitlement Cache**

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| Priority           | Critical                                                             |
| Component          | internal/entitlement/                                                |
| Description        | Resolve UserEntitlement on every inquiry before registry processing  |

```
ENTITLEMENT RESOLUTION FLOW
════════════════════════════

Inquiry arrives with Bearer token
    │
    ▼
Extract user_id from JWT / PAT / VAT claims
    │
    ▼
┌────────────────────────────────────┐
│  LAYER 1: sync.Map                 │
│  Key: user_id  Cost: ~50ns         │
│  HIT + TTL valid ──────────────────┼──► USE ENTITLEMENT
│  HIT + TTL expired → evict         │
│  MISS                              │
└────────────────────┬───────────────┘
                     │ MISS
                     ▼
┌────────────────────────────────────┐
│  LAYER 2: Redis GET                │
│  Key: user:entitlement:{user_id}   │
│  Cost: ~0.5ms                      │
│  HIT ──────────────────────────────┼──► warm sync.Map → USE
│  MISS                              │
└────────────────────┬───────────────┘
                     │ MISS
                     ▼
┌────────────────────────────────────┐
│  LAYER 3: POST /entitlement        │
│  to Central Server                 │
│  Cost: ~10-20ms                    │
│  Response ─────────────────────────┼──► SET Redis TTL:60s
│                                    │    SET sync.Map
│                                    │    USE ENTITLEMENT
└────────────────────────────────────┘
```

**SRS-FR-D-008: UserEntitlement Struct**

```go
type UserEntitlement struct {
    UserID         string
    OrgID          string
    AllowedPrompts []string   // FQN patterns: "sales/*", "chat_prompt:eng/debug:latest"
    AllowedSkills  []string   // skill names: ["summarize", "translate"]
    AllowedMCPs    []string   // MCP server names: ["github", "slack"]
    Permissions    []string   // RBAC tokens: ["gateway.prompt_registry.read"]
    BudgetLimitUSD float64
    RateLimitRPM   int
    RateLimitTPM   int
    DataScope      string     // own_data | team_data | all_data
    ExpiresAt      time.Time
}
```

### 4.4 Registry

**SRS-FR-D-009: Prompt Registry**

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| Priority           | High                                                                 |
| Component          | internal/registry/prompt/                                            |
| Description        | FQN resolution, template injection, guardrail union                  |
| Data source        | Local config cache — zero network calls                              |
| Entitlement check  | UserEntitlement.AllowedPrompts before resolution                     |

```
PROMPT RESOLUTION FLOW
══════════════════════

InquiryRequest.PromptFQN = "chat_prompt:sales/welcome:3"
    │
    ▼
Check AllowedPrompts contains "sales/*" or exact FQN
    │
NOT ALLOWED ──► HTTP 403 Forbidden
    │
ALLOWED
    ▼
Lookup local promptCache["sales"]["welcome"]["3"]
    │
NOT FOUND ──► HTTP 404 (prompt not in config)
    │
FOUND
    ▼
Inject template vars: {{customer_name}} → Variables["customer_name"]
    │
Missing required var ──► HTTP 400 Bad Request
    │
    ▼
Union guardrails:
  merged.Input  = prompt.InputGuardrails  + request.InputGuardrails
  merged.Output = prompt.OutputGuardrails + request.OutputGuardrails
    │
    ▼
Return resolved content + merged guardrail config
```

**SRS-FR-D-010: Skill Registry**

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| Priority           | High                                                                 |
| Component          | internal/registry/skill/                                             |
| Description        | SKILL.md catalog with progressive disclosure                         |
| Entitlement check  | UserEntitlement.AllowedSkills filter applied                         |

```
SKILL PROGRESSIVE DISCLOSURE
══════════════════════════════

STEP 1 — UPFRONT (with initial system prompt):
┌───────────────────────────────────────────────┐
│ Available skills:                             │
│  - summarize: Condense long text to key points│
│  - translate: Translate text to target lang   │
│  - code-review: Review code for quality issues│
└───────────────────────────────────────────────┘
Only name + description (<=200 chars) sent upfront.
Full SKILL.md body NOT included.

STEP 2 — ON LLM SELECTION (LLM picks a skill):
LLM response: { "skill_selected": "summarize" }
    │
    ▼
Inject full SKILL.md body into context:
┌───────────────────────────────────────────────┐
│ # Summarize Skill                             │
│ You are an expert summarizer...               │
│ ## Instructions                               │
│ 1. Extract key points                         │
│ 2. Preserve critical details                  │
│ [full SKILL.md body]                          │
└───────────────────────────────────────────────┘
```

**SRS-FR-D-011: MCP Registry**

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| Priority           | High                                                                 |
| Component          | internal/registry/mcp/                                               |
| Description        | Tool whitelist, HITL, Cedar/OPA, preload vs dynamic discovery        |
| Entitlement check  | UserEntitlement.AllowedMCPs filter applied                           |

```
MCP TOOL INVOCATION FLOW
══════════════════════════

LLM requests tool: "github.create_pr"
    │
    ▼
Check AllowedMCPs contains "github"
    │
NOT IN ALLOWED ──► error to LLM ("MCP server not permitted")
    │
IN ALLOWED
    ▼
Check tool "create_pr" in github.tools whitelist
    │
NOT IN WHITELIST ──► error to LLM ("tool not permitted")
    │
IN WHITELIST
    ▼
Run mcp_pre_invoke guardrail (Cedar/OPA policy evaluation)
    │
POLICY DENY ──► error to LLM
    │
POLICY ALLOW
    ▼
Is tool.Destructive == true?
  YES ──► emit tool.approval_required event
          Wait client approval (timeout: 5 min configurable)
            DENIED ──► error to LLM
            APPROVED ──► continue
  NO  ──► continue
    │
    ▼
Execute MCP tool call (HTTP/WebSocket to MCP server)
    │
    ▼
Run mcp_post_invoke guardrail (secrets / PII / code safety)
    │
    ▼
Return tool result to LLM
```

### 4.5 Policy

**SRS-FR-D-012: Guardrail Engine**

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| Priority           | Critical                                                             |
| Component          | internal/policy/guardrail/                                           |

```
GUARDRAIL HOOK ARCHITECTURE
════════════════════════════

Request pipeline positions:

  ┌─────────────────────────────────────────────────────────────┐
  │                                                             │
  │  CLIENT REQUEST                                             │
  │       │                                                     │
  │       ▼                                                     │
  │  ╔══════════════════════════════════════════╗               │
  │  ║ [HOOK 1] llm_input_guardrails            ║               │
  │  ║  Runs BEFORE forwarding to LLM           ║               │
  │  ║  Types: PII, prompt injection,           ║               │
  │  ║         content moderation, regex        ║               │
  │  ║  Validate mode: async (parallel w/ LLM)  ║               │
  │  ║  Mutate/Block mode: synchronous          ║               │
  │  ╚══════════════════════════════════════════╝               │
  │       │                                                     │
  │       ▼                                                     │
  │  LLM PROVIDER API                                           │
  │       │                                                     │
  │       ▼                                                     │
  │  ╔══════════════════════════════════════════╗               │
  │  ║ [HOOK 2] llm_output_guardrails           ║               │
  │  ║  Runs AFTER LLM response (stream:false)  ║               │
  │  ║  Types: PII redaction, secrets detect,   ║               │
  │  ║         content moderation               ║               │
  │  ║  Always synchronous                      ║               │
  │  ╚══════════════════════════════════════════╝               │
  │       │                                                     │
  │  MCP TOOL CALL (if agent mode)                              │
  │       │                                                     │
  │  ╔══════════════════════════════════════════╗               │
  │  ║ [HOOK 3] mcp_tool_pre_invoke             ║               │
  │  ║  Runs BEFORE MCP tool execution          ║               │
  │  ║  Types: SQL sanitization, Cedar/OPA,     ║               │
  │  ║         regex pattern match              ║               │
  │  ╚══════════════════════════════════════════╝               │
  │       │                                                     │
  │  MCP TOOL RESULT                                            │
  │       │                                                     │
  │  ╔══════════════════════════════════════════╗               │
  │  ║ [HOOK 4] mcp_tool_post_invoke            ║               │
  │  ║  Runs AFTER MCP tool result              ║               │
  │  ║  Types: secrets detection, PII redact,   ║               │
  │  ║         code safety linting              ║               │
  │  ╚══════════════════════════════════════════╝               │
  │                                                             │
  └─────────────────────────────────────────────────────────────┘

Enforcement modes:
  Enforce              → block request/response on violation
  Audit                → log only, no blocking
  Enforce-But-Degrade  → block unless guardrail provider unavailable

Operation modes:
  Validate → inspect content, no modification
  Mutate   → rewrite/redact content, continue
  Block    → reject request or response
```

**SRS-FR-D-013: Budget Policy**

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| Priority           | High                                                                 |
| Component          | internal/policy/budget/                                              |
| Description        | YAML ordered rules; first match controls allow/block; all tracked   |

```
BUDGET RULE EVALUATION
═══════════════════════

Rules (evaluated top-to-bottom):
┌───────────────────────────────────────────────────────┐
│ rule[0]: when user=alice@x.com  limit=$10/day         │
│ rule[1]: when team=engineering  limit=$500/day        │
│ rule[2]: when *                 limit=$5000/day       │
└───────────────────────────────────────────────────────┘

Request from alice@x.com (team=engineering):

rule[0] matches → FIRST MATCH → controls allow/block
  Redis key: budget:rule0:alice@x.com:2026-06-04
  INCRBYFLOAT by estimated_cost
  current=$8.50 < limit=$10 → ALLOW

rule[1] matches → tracking only (not first match)
  Redis key: budget:rule1:engineering:2026-06-04
  INCRBYFLOAT (tracking, does not control)

rule[2] matches → tracking only
  Redis key: budget:rule2:all:2026-06-04
  INCRBYFLOAT (tracking, does not control)

If rule[0] counter >= $10:
  → HTTP 429: {"error": "budget_exceeded", "rule_id": "rule0"}
  → Do NOT call LLM provider

Budget units: cost_per_hour | cost_per_day | cost_per_week | cost_per_month
Applies per:  user | model | virtualaccount | metadata.<key>
Audit mode:   requests allowed through; tracking + alerts still active
```

**SRS-FR-D-014: Rate Limit Policy**

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| Priority           | High                                                                 |
| Component          | internal/policy/ratelimit/                                           |
| Description        | Sliding window token bucket via Redis Lua atomic scripts             |

```
SLIDING WINDOW RATE LIMIT
══════════════════════════

Window: 60 seconds  Bucket size: 5 seconds  Buckets: 12

Timeline buckets (5s each):
t=0  t=5  t=10  t=15  t=20  t=25  t=30  t=35  t=40  t=45  t=50  t=55
[B0] [B1] [B2]  [B3]  [B4]  [B5]  [B6]  [B7]  [B8]  [B9] [B10] [B11]

At t=62: window slides → drop B0, count B1..B11 + new B12
Total = sum of all buckets in window
If total >= limit → HTTP 429 + Retry-After header

Redis Keys:
  ratelimit:{rule_id}:{entity}:{bucket_ts}
  Type: String (integer counter)
  TTL:  65 seconds (5s buffer)

Redis Lua Script (atomic check-and-increment):
  1. Compute current bucket = floor(now / 5) * 5
  2. INCR current bucket key
  3. Sum all bucket keys in [now-60s .. now]
  4. If sum > limit → return REJECT (0)
  5. Else → return ALLOW (1)

Rate limit units: requests_per_minute | requests_per_hour | requests_per_day
                  tokens_per_minute   | tokens_per_hour   | tokens_per_day
Applies per:      user | model | virtualaccount | metadata.<key>
Error response:   HTTP 429 with Retry-After: {seconds} header
```

### 4.6 LLM Dispatch

**SRS-FR-D-015: LLM Provider Dispatch**

| Field      | Value                                                                        |
|------------|------------------------------------------------------------------------------|
| Priority   | Critical                                                                     |
| Component  | internal/llm/                                                                |

| Step | Action                                                                 |
|------|------------------------------------------------------------------------|
| 1    | Serialize InquiryRequest to provider-specific payload                  |
| 2    | Set API key from config (AES-256 decrypted in memory, never logged)    |
| 3    | POST to provider endpoint with configurable timeout                    |
| 4    | On provider error: circuit breaker check (5 failures/10s → open)      |
| 5    | Run llm_output_guardrails on response                                  |
| 6    | Return normalized response to client                                   |
| 7    | Publish TransactionEvent to Kafka (fire and forget — non-blocking)     |

**SRS-FR-D-016: Kafka Producer (Fire and Forget)**

| Field      | Value                                                                        |
|------------|------------------------------------------------------------------------------|
| Priority   | High                                                                         |
| Component  | internal/producer/                                                           |
| Behavior   | Publish TransactionEvent after response returned to user                     |
| Non-blocking | Daemon does NOT wait for Kafka ACK on hot path                            |
| Retry      | Failed publishes retried by background producer worker with backoff          |

---

## 5. Central Server Requirements

### 5.1 Server Overview

```
CENTRAL SERVER INTERNAL STRUCTURE
═══════════════════════════════════

┌─────────────────────────────────────────────────────────────────────┐
│                        CENTRAL SERVER                               │
│                                                                     │
│  HTTP ENDPOINTS              │  BACKGROUND WORKERS                  │
│  ───────────────             │  ──────────────────                  │
│  POST /health                │  Kafka Consumer                      │
│  POST /config                │    group: central-server-consumer    │
│  POST /entitlement           │    topic: llm.transactions           │
│  POST /admin/config          │    on message:                       │
│  POST /admin/entitlement     │      validate schema                 │
│  POST /admin/invalidate      │      enrich (pricing lookup)         │
│                              │      INSERT request_logs             │
│                              │      INSERT cost_records             │
│                              │      INSERT audit_log                │
│                              │      COMMIT kafka offset             │
│                                                                     │
│  REPOSITORY LAYER (pgx)                                             │
│  ─────────────────────                                              │
│  users · orgs · api_keys · roles · permissions                      │
│  user_entitlements · prompt_versions · skills                       │
│  mcp_servers · budget_rules · rate_limit_rules                      │
│  guardrail_policies · request_logs · cost_records                   │
│  agent_sessions · audit_log · auth_events                           │
│                                                                     │
│  PUBLISHER                                                          │
│  ─────────                                                          │
│  Redis PUBLISH "user:invalidate" {user_id}                          │
│  Called on: entitlement change, role assignment, account suspend    │
└─────────────────────────────────────────────────────────────────────┘
```

**SRS-FR-S-001: Health Endpoint**

| Field      | Value                                                                        |
|------------|------------------------------------------------------------------------------|
| Priority   | Critical                                                                     |
| Endpoint   | POST /health                                                                 |
| Latency    | Must respond < 500ms                                                         |
| Response   | HTTP 200: `{"status":"ok","server_id":"...","timestamp":"ISO8601"}`          |

**SRS-FR-S-002: Config Endpoint**

| Field      | Value                                                                        |
|------------|------------------------------------------------------------------------------|
| Priority   | Critical                                                                     |
| Endpoint   | POST /config                                                                 |
| Auth       | Bearer daemon_api_key                                                        |
| Response   | 200 + full GatewayConfig JSON; 304 if version unchanged                      |
| Content    | proxies, registry (prompts/skills/MCPs), policies (guardrails/budget/rate)  |

**SRS-FR-S-003: Entitlement Endpoint**

| Field      | Value                                                                        |
|------------|------------------------------------------------------------------------------|
| Priority   | Critical                                                                     |
| Endpoint   | POST /entitlement                                                            |
| Auth       | Bearer daemon_api_key                                                        |
| Latency    | P95 < 50ms                                                                   |
| Response   | 200 + UserEntitlement JSON; 404 if user not found                            |

**SRS-FR-S-004: Admin Entitlement + Invalidation**

| Field      | Value                                                                        |
|------------|------------------------------------------------------------------------------|
| Priority   | Critical                                                                     |
| Endpoint   | POST /admin/entitlement                                                      |

```
ADMIN ENTITLEMENT CHANGE FLOW
═══════════════════════════════

Admin calls: POST /admin/entitlement
Body: { "user_id": "uuid", "allowed_skills": ["summarize"] }
    │
    ▼
UPDATE user_entitlements
  SET allowed_skills = '["summarize"]'
  WHERE user_id = ?
    │
    ▼
Redis PUBLISH "user:invalidate" "{user_id}"
    │
    ├──► Daemon instance 1 evicts sync.Map + Redis key
    ├──► Daemon instance 2 evicts sync.Map + Redis key
    └──► Daemon instance N evicts sync.Map + Redis key

Next request from user:
  cache miss → POST /entitlement → fresh entitlement applied
  Max lag: one request cycle (~20ms worst case)
```

**SRS-FR-S-005: Kafka Consumer**

| Field      | Value                                                                        |
|------------|------------------------------------------------------------------------------|
| Priority   | High                                                                         |
| Component  | internal/server/consumer/                                                    |
| Topic      | llm.transactions                                                             |
| Group      | central-server-consumer                                                      |
| Delivery   | At-least-once; idempotent via ON CONFLICT DO NOTHING                         |

```
KAFKA CONSUMER FLOW
════════════════════

topic: llm.transactions
    │
    ▼
Deserialize TransactionEvent JSON
    │
    ▼
Validate required fields (request_id, org_id, model, status)
    │
INVALID ──► log error, commit offset, skip
    │
VALID
    ▼
Enrich: lookup model pricing from Redis/config cache
    │
    ▼
INSERT INTO request_logs (...) ON CONFLICT (id) DO NOTHING
    │
    ▼
INSERT INTO cost_records (...) ON CONFLICT (request_log_id) DO NOTHING
    │
    ▼
Commit Kafka offset
    │
    ▼
(Optional) Emit budget alert if threshold crossed (75/90/95/100%)
```

---

## 6. Message Contracts

### 6.1 Message Flow Overview

```
MESSAGE FLOW DIAGRAM
════════════════════

DAEMON                         CENTRAL SERVER              REDIS
  │                                  │                       │
  │── POST /health ─────────────────►│                       │
  │◄─ 200 OK ───────────────────────│                       │
  │                                  │                       │
  │── POST /config (every N sec) ───►│                       │
  │   Body: {gateway_id, version}    │                       │
  │◄─ 200 GatewayConfig / 304 ──────│                       │
  │                                  │                       │
  │── POST /entitlement (cache miss)►│                       │
  │   Body: {user_id, org_id}        │                       │
  │◄─ 200 UserEntitlement ──────────│                       │
  │                                  │                       │
  │   SUBSCRIBE ─────────────────────┼──────────────────────►│
  │◄──user:invalidate ───────────────┼──────────────────────│
  │   (on admin change)              │                       │
  │                                  │◄─ PUBLISH ───────────│
  │                                  │   (admin trigger)     │
  │                                  │                       │
KAFKA                                │                       │
  │                                  │                       │
  │◄─ TransactionEvent (daemon) ─────│                       │
  │   topic: llm.transactions        │                       │
  │─────────────────────────────────►│ (Kafka consumer)      │
  │                                  │── INSERT PostgreSQL   │
  │                                  │                       │
```

### 6.2 GatewayConfig Message Schema

| Field                          | Type    | Description                                       |
|--------------------------------|---------|---------------------------------------------------|
| version                        | string  | SHA-256 of config content; used for change detect |
| gateway_id                     | string  | Daemon instance identifier                        |
| proxies[].provider             | string  | openai \| claude \| moonshot \| ollama            |
| proxies[].port                 | int     | Listening port for this provider                  |
| proxies[].base_url             | string  | Provider API base URL                             |
| proxies[].api_key_enc          | string  | AES-256-GCM encrypted API key                     |
| registry.prompts[].fqn         | string  | chat_prompt:{repo}/{name}:{version}               |
| registry.prompts[].content     | string  | Prompt template body                              |
| registry.prompts[].variables   | array   | Required template variable names                  |
| registry.skills[].name         | string  | Unique skill name                                 |
| registry.skills[].description  | string  | <= 200 characters (shown upfront)                 |
| registry.skills[].content      | string  | Full SKILL.md body (injected on select)           |
| registry.skills[].preload      | bool    | Always include in context if true                 |
| registry.mcps[].name           | string  | MCP server logical name                           |
| registry.mcps[].url            | string  | MCP server URL                                    |
| registry.mcps[].tools[]        | array   | Tool definitions with destructive flag            |
| policies.guardrails[]          | array   | Guardrail rule definitions                        |
| policies.budget_rules[]        | array   | Ordered budget YAML rules                         |
| policies.rate_rules[]          | array   | Ordered rate limit YAML rules                     |

### 6.3 TransactionEvent (Kafka Message)

| Field           | Type    | Description                                              |
|-----------------|---------|----------------------------------------------------------|
| event_id        | UUID    | Idempotency key (ON CONFLICT DO NOTHING)                 |
| request_id      | UUID    | Unique request identifier                                |
| gateway_id      | string  | Originating daemon instance                              |
| org_id          | UUID    | Organisation                                             |
| user_id         | UUID    | Requesting user (nullable for VAT)                       |
| provider        | string  | openai \| claude \| moonshot \| ollama                   |
| model           | string  | Actual model name used                                   |
| status          | string  | success \| error                                         |
| latency_ms      | int     | Total request latency                                    |
| ttft_ms         | int     | Time to first token (streaming)                          |
| input_tokens    | int     | Input token count                                        |
| output_tokens   | int     | Output token count                                       |
| cost_usd        | float64 | Calculated cost in USD                                   |
| cache_status    | string  | hit \| miss \| none                                      |
| prompt_fqn      | string  | Resolved prompt FQN if used (nullable)                   |
| skills_used     | array   | Skill names invoked                                      |
| mcps_called     | array   | MCP tool calls made                                      |
| guardrails_hit  | array   | Guardrail IDs that fired                                 |
| metadata        | object  | Client-supplied X-TFY-METADATA                           |
| created_at      | ISO8601 | Request completion timestamp                             |

### 6.4 Config Poll Protocol

```
CONFIG POLL PROTOCOL
═════════════════════

Every POLL_INTERVAL_SEC seconds:

  Daemon sends:
  ┌────────────────────────────────────────┐
  │  POST /config                          │
  │  Authorization: Bearer {daemon_key}    │
  │  Body: {                               │
  │    "gateway_id": "gw-001",             │
  │    "current_version": "abc123def"      │
  │  }                                     │
  └────────────────────────────────────────┘

  Server responds:
  ┌────────────────────────────────────────┐
  │  304 Not Modified                      │  ← config unchanged
  │  (no body)                             │
  │                                        │
  │  OR                                    │
  │                                        │
  │  200 OK                                │  ← config changed
  │  Body: full GatewayConfig JSON         │
  └────────────────────────────────────────┘

  Daemon on 304: no action (keep cached config)
  Daemon on 200: diff → if changed → send to configChan
  Daemon on 5xx/timeout: log warning, keep cached config
```

---

## 7. Platform Infrastructure

### 7.1 Redis

```
REDIS KEY SCHEMA
════════════════

Entitlement Cache:
  Key:     user:entitlement:{user_id}
  Type:    String (JSON serialized UserEntitlement)
  TTL:     60 seconds
  Set by:  Daemon Layer 3 fetch from Central Server
  DEL by:  Daemon on user:invalidate pub/sub

Rate Limit Buckets:
  Key:     ratelimit:{rule_id}:{entity}:{bucket_ts}
  Type:    String (integer counter)
  TTL:     65 seconds (5s bucket + 5s buffer)
  INCR by: Redis Lua script (atomic)

Budget Counters:
  Key:     budget:{rule_id}:{entity}:{period}
  Type:    String (float, stored as string)
  TTL:     Until period_end + 1 hour
  INCR by: Redis INCRBYFLOAT (atomic)
  Example: budget:rule0:usr-alice:2026-06-04

Exact Prompt Cache:
  Key:     cache:exact:{sha256_of_request}
  Type:    String (JSON response)
  TTL:     Configurable 1-86400s

Semantic Prompt Cache:
  Key:     cache:semantic:emb:{sha256}
  Type:    String (serialized embedding vector)
  TTL:     3600s default

Agent Session State:
  Key:     session:{execution_id}
  Type:    Hash
  TTL:     86400s (24h)

Pub/Sub:
  Channel:   user:invalidate
  Message:   {user_id}
  Publisher: Central Server (on entitlement change)
  Subscriber: All Daemon instances
```

**SRS-FR-P-001: Redis Configuration**

| Environment | Mode         | Nodes                    | TLS      |
|-------------|--------------|--------------------------|----------|
| Development | Standalone   | 1 node                   | Optional |
| Windows Dev | Standalone   | 1 node local install     | No       |
| Production  | Cluster      | 3 primary + 3 replica    | Required |

Connection pool: min=5, max=50. Auto-reconnect on failure.

### 7.2 Kafka

```
KAFKA TOPIC DESIGN
══════════════════

Topic: llm.transactions
  Partitions:  12  (scale with daemon instances)
  Replication: 3   (production); 1 (dev)
  Retention:   7 days
  Compression: lz4
  Key:         org_id (ensures per-org ordering)
  Value:       TransactionEvent JSON

Topic: llm.audit (optional — for extended audit trail)
  Partitions:  6
  Replication: 3
  Retention:   30 days
  Key:         user_id

Consumer Group: central-server-consumer
  Delivery:    at-least-once
  Offset commit: AFTER successful DB INSERT (not before)
  Idempotency: event_id as unique key (ON CONFLICT DO NOTHING)
```

**SRS-FR-P-002: Kafka Delivery**

| Requirement       | Setting                                        |
|-------------------|------------------------------------------------|
| Producer acks     | acks=all (all ISR must confirm)                |
| Consumer offset   | Committed AFTER DB insert (not before)         |
| Idempotency key   | TransactionEvent.event_id                      |
| Error handling    | Dead letter topic after 3 retry failures       |

### 7.3 PostgreSQL

```
POSTGRESQL DEPLOYMENT
══════════════════════

Production:
  Primary ──WAL──► Replica 1 (hot standby, failover < 30s)
                 ► Replica 2 (read-only for analytics queries)
  Failover: Patroni + etcd
  Pooler:   PgBouncer (max 100 server connections, pool_mode=transaction)
  Partitioning: request_logs PARTITION BY RANGE (created_at) — monthly

Development / Windows:
  Single PostgreSQL 16 instance
  Connection string via DATABASE_URL env var
  No TLS required locally

Migrations:
  Tool: golang-migrate (https://github.com/golang-migrate/migrate)
  Dir:  /migrations/*.sql
  Run:  Central Server on startup
  Format: {NNNNNN}_{description}.up.sql / .down.sql
```

### 7.4 Docker Infrastructure Server

```
SEPARATE DOCKER INFRASTRUCTURE SERVER
═══════════════════════════════════════

Infrastructure host (docker-compose.infra.yml):
┌──────────────────────────────────────────────────────────────┐
│                                                              │
│  postgres:16-alpine         redis:7-alpine                   │
│  port: 5432                 port: 6379                       │
│  volume: pgdata             command: --maxmemory 512mb       │
│                                      --maxmemory-policy lru  │
│                                                              │
│  kafka (confluentinc/cp-kafka:7.6.0)                        │
│  port: 9092                                                  │
│                                                              │
│  zookeeper                                                   │
│  port: 2181                                                  │
│                                                              │
│  network: pangreksa-net (bridge)                             │
└──────────────────────────────────────────────────────────────┘

Application host (docker-compose.app.yml):
┌──────────────────────────────────────────────────────────────┐
│                                                              │
│  central-server                                              │
│  image: pangreksa/central-server:latest                      │
│  port: 9000                                                  │
│  env: DATABASE_URL, REDIS_URL, KAFKA_BROKERS                 │
│                                                              │
│  gateway-daemon                                              │
│  image: pangreksa/gateway-daemon:latest                      │
│  ports: 8080-8083 (one per provider)                         │
│  env: CONFIG_SERVER_URL, REDIS_URL, KAFKA_BROKERS            │
│                                                              │
│  network: pangreksa-net (external — connects to infra host)  │
└──────────────────────────────────────────────────────────────┘
```

**SRS-FR-P-003: Windows Local Testing**

| Requirement    | Detail                                                              |
|----------------|---------------------------------------------------------------------|
| Build          | go build ./cmd/daemon and go build ./cmd/server on windows/amd64   |
| Test           | go test ./... -race passes on Windows                               |
| Config         | All settings via environment variables (no /etc/ paths)            |
| Infrastructure | Local PostgreSQL 16, Redis 7, Kafka — or Docker Desktop            |
| Path handling  | filepath.Join used throughout; no Unix path assumptions            |

---

## 8. Non-Functional Requirements

| ID          | Quality Attribute | Requirement                              | Metric / Target                                |
|-------------|-------------------|------------------------------------------|------------------------------------------------|
| SRS-NFR-001 | Performance       | Gateway hot path overhead                | P95 < 50ms excluding LLM provider latency      |
| SRS-NFR-002 | Performance       | Entitlement resolution — cache hit       | P99 < 1ms (sync.Map lookup)                    |
| SRS-NFR-003 | Performance       | Entitlement resolution — Redis hit       | P95 < 2ms                                      |
| SRS-NFR-004 | Performance       | Config poll response from server         | P95 < 100ms                                    |
| SRS-NFR-005 | Performance       | /entitlement endpoint latency            | P95 < 50ms                                     |
| SRS-NFR-006 | Throughput        | Daemon requests per second               | 5,000 RPS per daemon instance                  |
| SRS-NFR-007 | Scalability       | Horizontal scale-out                     | Stateless daemon; linear scale with instances  |
| SRS-NFR-008 | Availability      | Daemon uptime SLA                        | 99.9% (< 8.77 hours/year downtime)             |
| SRS-NFR-009 | Reliability       | Transaction delivery to PostgreSQL       | At-least-once via Kafka; idempotent consumer   |
| SRS-NFR-010 | Reliability       | Config continuity when server is down    | Daemon serves indefinitely from cached config  |
| SRS-NFR-011 | Reliability       | Entitlement on Redis failure             | Fallback to Central Server direct fetch        |
| SRS-NFR-012 | Security          | All secrets encrypted                    | AES-256-GCM; keys in env vars or Vault         |
| SRS-NFR-013 | Security          | Transport encryption                     | TLS 1.3 for all HTTP in production             |
| SRS-NFR-014 | Security          | No secrets in source or images           | Gitleaks in CI pipeline; env vars only         |
| SRS-NFR-015 | Maintainability   | Test coverage                            | >= 80% unit + integration coverage             |
| SRS-NFR-016 | Observability     | Distributed tracing                      | OpenTelemetry; 100% of requests traced         |
| SRS-NFR-017 | Observability     | Structured logging                       | JSON logs (zerolog or zap); log level via env  |
| SRS-NFR-018 | Observability     | Metrics                                  | Prometheus /metrics endpoint on each service   |
| SRS-NFR-019 | Portability       | Windows local testing                    | go build/test pass on windows/amd64            |
| SRS-NFR-020 | Portability       | Docker image delivery                    | Multi-stage Dockerfile; linux/amd64 image      |
| SRS-NFR-021 | Compliance        | Immutable audit log                      | INSERT-only audit_log; REVOKE UPDATE/DELETE    |

---

## 9. API Contracts

### 9.1 Daemon Proxy Endpoints

```
POST :{provider_port}/v1/chat/completions    (OpenAI-compatible)
POST :{provider_port}/v1/messages           (Anthropic-compatible)

Authorization: Bearer <PAT | VAT | JWT>
Content-Type: application/json

Request: Provider-native format (passed through from client)
Response: Provider-native format (passed through from LLM)

Response Headers added by daemon:
  X-Gateway-Request-ID: {uuid}
  X-Gateway-Latency-Ms: {ms}
  X-Cache-Status: hit | miss | none
  X-Guardrails-Applied: {comma-separated guardrail IDs}
```

### 9.2 Central Server Internal API

```
POST /health
  Response 200: { "status": "ok", "server_id": "...", "timestamp": "ISO8601" }

────────────────────────────────────────────────────────────────
POST /config
  Authorization: Bearer {DAEMON_API_KEY}
  Body: { "gateway_id": "gw-001", "current_version": "sha256" }

  Response 200: GatewayConfig JSON (full config)
  Response 304: (no body — config version unchanged)
  Response 401: { "error": "unauthorized" }

────────────────────────────────────────────────────────────────
POST /entitlement
  Authorization: Bearer {DAEMON_API_KEY}
  Body: { "user_id": "uuid", "org_id": "uuid" }

  Response 200: UserEntitlement JSON
  Response 404: { "error": "user_not_found" }
  Response 401: { "error": "unauthorized" }

────────────────────────────────────────────────────────────────
POST /admin/config
  Authorization: Bearer {ADMIN_API_KEY}
  Body: partial or full GatewayConfig

  Response 200: { "version": "new_sha256", "applied_at": "ISO8601" }
  Response 400: { "error": "validation_failed", "details": [...] }

────────────────────────────────────────────────────────────────
POST /admin/entitlement
  Authorization: Bearer {ADMIN_API_KEY}
  Body: { "user_id": "uuid", "changes": { "allowed_skills": [...] } }

  Response 200: { "updated": true, "invalidated": true }
  Response 404: { "error": "user_not_found" }

────────────────────────────────────────────────────────────────
POST /admin/invalidate
  Authorization: Bearer {ADMIN_API_KEY}
  Body: { "user_id": "uuid" }

  Response 200: { "published": true, "channel": "user:invalidate" }
```

---

## 10. Data Architecture & ERD

### 10.1 Data Strategy

| Concern          | Solution                                                           |
|------------------|--------------------------------------------------------------------|
| Primary store    | PostgreSQL 16 (pgx/v5 driver)                                      |
| Migrations       | golang-migrate — versioned SQL files                               |
| Caching          | Redis — TTL-based; write-through on entitlement fetch              |
| Async writes     | Kafka producer -> consumer -> PostgreSQL INSERT                    |
| Backup           | pg_dump daily; WAL archiving to S3/MinIO                           |
| Partitioning     | request_logs PARTITION BY RANGE (created_at) — monthly             |

### 10.2 Entity Summary

| Entity               | Description                                               |
|----------------------|-----------------------------------------------------------|
| organizations        | Top-level tenant; all data scoped per org                 |
| users                | Human users (local / Entra ID / Active Directory)         |
| virtual_accounts     | Service identity (VAT bearer)                             |
| api_keys             | PAT + VAT token hashes                                    |
| roles                | Named permission bundles                                  |
| permissions          | Fine-grained permission tokens (e.g. gateway.mcp.read)    |
| user_roles           | Many-to-many: users <-> roles                             |
| role_permissions     | Many-to-many: roles <-> permissions                       |
| user_entitlements    | Per-user allowed prompts / skills / MCPs                  |
| provider_accounts    | LLM provider credentials (AES-256 encrypted)              |
| virtual_models       | Logical model abstraction with routing config (JSONB)     |
| prompt_repositories  | Prompt namespace (repo)                                   |
| prompt_versions      | Versioned prompt content — immutable after creation       |
| skills               | SKILL.md catalog entries with frontmatter                 |
| mcp_servers          | Registered MCP servers with tool definitions              |
| guardrail_policies   | Guardrail rule definitions per org                        |
| budget_rules         | YAML budget rule definitions (ordered)                    |
| rate_limit_rules     | YAML rate limit rule definitions (ordered)                |
| request_logs         | Partitioned monthly LLM request audit trail               |
| cost_records         | Per-request cost attribution (1:1 with request_logs)      |
| agent_sessions       | Agent execution state (TTL 24h)                           |
| audit_log            | Immutable mutation log (INSERT-only)                      |
| auth_events          | Authentication event log                                  |

### 10.3 ERD (ASCII)

```
ERD — PANGREKSA AI GATEWAY ENGINE
════════════════════════════════════════════════════════════════════════

organizations (1) ────────────────────────────────────────────────────────────────────┐
  ├── id UUID PK                                                                       │
  ├── name VARCHAR(255)                                                                │
  ├── slug VARCHAR(100) UNIQUE                                                         │
  └── created_at TIMESTAMPTZ                                                           │
                                                                                       │
  ┌──────────────────────────────────────────────────────────────────────(FK org_id)──┘
  │
  ├──(many)──► users
  │              ├── id UUID PK
  │              ├── org_id FK
  │              ├── email VARCHAR(320) UNIQUE per org
  │              ├── password_hash (nullable for SSO)
  │              ├── provider (local|entra|ad)
  │              ├── status (active|locked|suspended)
  │              ├── mfa_enabled BOOLEAN
  │              ├── failed_login_count INT
  │              └── locked_until TIMESTAMPTZ
  │
  │                  users (1) ──(many)──► user_roles ◄──(many)── roles
  │                                           ├── user_id FK           ├── id UUID PK
  │                                           ├── role_id FK           ├── org_id FK
  │                                           ├── assigned_by FK       ├── name
  │                                           └── assigned_at          └── is_system_role
  │
  │                  roles (1) ──(many)──► role_permissions ◄──(many)── permissions
  │                                            ├── role_id FK              ├── id UUID PK
  │                                            └── permission_id FK        ├── token (e.g.
  │                                                                         │   gateway.mcp.read)
  │                                                                         └── description
  │
  │                  users (1) ──(1)──► user_entitlements
  │                                        ├── id UUID PK
  │                                        ├── user_id FK UNIQUE
  │                                        ├── org_id FK
  │                                        ├── allowed_prompts JSONB
  │                                        ├── allowed_skills  JSONB
  │                                        ├── allowed_mcps    JSONB
  │                                        ├── budget_limit_usd DECIMAL
  │                                        ├── rate_limit_rpm INT
  │                                        ├── rate_limit_tpm INT
  │                                        ├── data_scope VARCHAR
  │                                        └── updated_at TIMESTAMPTZ
  │
  ├──(many)──► virtual_accounts
  │              ├── id UUID PK
  │              ├── org_id FK
  │              ├── name
  │              ├── token_hash
  │              └── auto_rotate BOOLEAN
  │
  │                  users + virtual_accounts ──(many)──► api_keys
  │                                                          ├── id UUID PK
  │                                                          ├── user_id FK (nullable)
  │                                                          ├── va_id FK (nullable)
  │                                                          ├── token_hash
  │                                                          ├── name
  │                                                          └── expires_at
  │
  ├──(many)──► provider_accounts
  │              ├── id UUID PK
  │              ├── org_id FK
  │              ├── provider_name
  │              ├── api_key_enc (AES-256)
  │              ├── base_url
  │              ├── region
  │              └── status
  │
  ├──(many)──► virtual_models
  │              ├── id UUID PK
  │              ├── org_id FK
  │              ├── name
  │              └── routing_config JSONB
  │
  ├──(many)──► prompt_repositories
  │              ├── id UUID PK
  │              ├── org_id FK
  │              └── name UNIQUE per org
  │
  │                  prompt_repositories (1) ──(many)──► prompt_versions
  │                                                          ├── id UUID PK
  │                                                          ├── repo_id FK
  │                                                          ├── prompt_name
  │                                                          ├── version INT (auto-increment)
  │                                                          ├── content TEXT (immutable)
  │                                                          ├── config JSONB
  │                                                          └── created_by FK -> users
  │
  ├──(many)──► skills
  │              ├── id UUID PK
  │              ├── org_id FK
  │              ├── name UNIQUE per org+version
  │              ├── description VARCHAR(200)
  │              ├── version INT
  │              ├── content TEXT (SKILL.md body)
  │              ├── frontmatter JSONB
  │              └── preload BOOLEAN
  │
  ├──(many)──► mcp_servers
  │              ├── id UUID PK
  │              ├── org_id FK
  │              ├── name UNIQUE per org
  │              ├── url
  │              ├── auth_type (oauth2|api_key|none)
  │              ├── credentials_enc (AES-256)
  │              ├── tools JSONB (tool definitions + destructive flag)
  │              └── status
  │
  ├──(many)──► guardrail_policies
  │              ├── id UUID PK
  │              ├── org_id FK
  │              ├── type (pii|injection|content|secrets|sql|code|regex)
  │              ├── hook (llm_input|llm_output|mcp_pre|mcp_post)
  │              ├── mode (validate|mutate|block)
  │              ├── enforcement (enforce|audit|degrade)
  │              └── config JSONB
  │
  ├──(many)──► budget_rules
  │              ├── id UUID PK
  │              ├── org_id FK
  │              ├── priority INT (order of evaluation)
  │              └── rule_yaml TEXT
  │
  ├──(many)──► rate_limit_rules
  │              ├── id UUID PK
  │              ├── org_id FK
  │              ├── priority INT
  │              └── rule_yaml TEXT
  │
  ├──(many)──► request_logs (PARTITIONED BY RANGE created_at)
  │              ├── id UUID (PK composite with created_at)
  │              ├── org_id FK
  │              ├── trace_id VARCHAR(64)
  │              ├── user_id FK -> users (nullable)
  │              ├── virtual_acct_id FK -> virtual_accounts (nullable)
  │              ├── model
  │              ├── provider
  │              ├── status (success|error)
  │              ├── latency_ms INT
  │              ├── ttft_ms INT
  │              ├── input_tokens INT
  │              ├── output_tokens INT
  │              ├── cost_usd DECIMAL(12,8)
  │              ├── cache_status (hit|miss|none)
  │              ├── prompt_fqn VARCHAR(512)
  │              ├── skills_used JSONB
  │              ├── mcps_called JSONB
  │              ├── guardrails_hit JSONB
  │              ├── metadata JSONB
  │              └── created_at TIMESTAMPTZ
  │
  │                  request_logs (1) ──(1)──► cost_records
  │                                               ├── id UUID PK
  │                                               ├── org_id FK
  │                                               ├── request_log_id FK UNIQUE
  │                                               ├── user_id FK
  │                                               ├── model
  │                                               ├── input_tokens INT
  │                                               ├── output_tokens INT
  │                                               ├── cost_usd DECIMAL(12,8)
  │                                               ├── period_day DATE
  │                                               └── period_month VARCHAR(7)
  │
  ├──(many)──► agent_sessions
  │              ├── id UUID PK
  │              ├── org_id FK
  │              ├── user_id FK
  │              ├── execution_id UNIQUE
  │              ├── response_id
  │              ├── state JSONB
  │              ├── created_at TIMESTAMPTZ
  │              └── ttl_expires_at TIMESTAMPTZ
  │
  ├──(many)──► audit_log  [INSERT-ONLY — REVOKE UPDATE/DELETE]
  │              ├── id UUID PK
  │              ├── org_id FK
  │              ├── user_id FK (nullable)
  │              ├── ip_address INET
  │              ├── event_type VARCHAR(100)
  │              ├── resource_type
  │              ├── resource_id UUID
  │              ├── old_value JSONB
  │              ├── new_value JSONB
  │              └── created_at TIMESTAMPTZ
  │
  └──(many)──► auth_events
                 ├── id UUID PK
                 ├── org_id FK
                 ├── user_id FK (nullable)
                 ├── provider (local|entra|ad)
                 ├── event_type (login|logout|failure|mfa|lockout)
                 ├── success BOOLEAN
                 ├── failure_reason
                 ├── ip_address INET
                 └── created_at TIMESTAMPTZ
```

### 10.4 Critical DDL

```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE organizations (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(255) NOT NULL,
    slug       VARCHAR(100) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE users (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email               VARCHAR(320) NOT NULL,
    password_hash       VARCHAR(255),
    provider            VARCHAR(50)  NOT NULL DEFAULT 'local',
    status              VARCHAR(20)  NOT NULL DEFAULT 'active',
    mfa_enabled         BOOLEAN      NOT NULL DEFAULT FALSE,
    failed_login_count  INT          NOT NULL DEFAULT 0,
    locked_until        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, email)
);
CREATE INDEX idx_users_org_email ON users(org_id, email);

CREATE TABLE user_entitlements (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID        NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    org_id           UUID        NOT NULL REFERENCES organizations(id),
    allowed_prompts  JSONB       NOT NULL DEFAULT '[]',
    allowed_skills   JSONB       NOT NULL DEFAULT '[]',
    allowed_mcps     JSONB       NOT NULL DEFAULT '[]',
    budget_limit_usd DECIMAL(12,4) NOT NULL DEFAULT 0,
    rate_limit_rpm   INT          NOT NULL DEFAULT 60,
    rate_limit_tpm   INT          NOT NULL DEFAULT 100000,
    data_scope       VARCHAR(20)  NOT NULL DEFAULT 'own_data',
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE request_logs (
    id              UUID          NOT NULL DEFAULT gen_random_uuid(),
    org_id          UUID          NOT NULL,
    trace_id        VARCHAR(64),
    span_id         VARCHAR(32),
    user_id         UUID,
    virtual_acct_id UUID,
    model           VARCHAR(255)   NOT NULL,
    provider        VARCHAR(100)   NOT NULL,
    virtual_model   VARCHAR(255),
    status          VARCHAR(20)    NOT NULL,
    http_status     SMALLINT,
    latency_ms      INTEGER,
    ttft_ms         INTEGER,
    input_tokens    INTEGER        NOT NULL DEFAULT 0,
    output_tokens   INTEGER        NOT NULL DEFAULT 0,
    cost_usd        DECIMAL(12,8)  NOT NULL DEFAULT 0,
    cache_status    VARCHAR(10),
    prompt_fqn      VARCHAR(512),
    skills_used     JSONB          NOT NULL DEFAULT '[]',
    mcps_called     JSONB          NOT NULL DEFAULT '[]',
    guardrails_hit  JSONB          NOT NULL DEFAULT '[]',
    conversation_id VARCHAR(255),
    execution_id    VARCHAR(64),
    metadata        JSONB          NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Create monthly partitions (automate via pg_partman in production)
CREATE TABLE request_logs_2026_06
    PARTITION OF request_logs
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE TABLE cost_records (
    id              UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID           NOT NULL REFERENCES organizations(id),
    request_log_id  UUID           NOT NULL UNIQUE,
    user_id         UUID           REFERENCES users(id),
    model           VARCHAR(255)   NOT NULL,
    input_tokens    INTEGER        NOT NULL DEFAULT 0,
    output_tokens   INTEGER        NOT NULL DEFAULT 0,
    cost_usd        DECIMAL(12,8)  NOT NULL DEFAULT 0,
    period_day      DATE           NOT NULL,
    period_month    VARCHAR(7)     NOT NULL
);

CREATE TABLE audit_log (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID         NOT NULL,
    user_id       UUID,
    ip_address    INET,
    user_agent    TEXT,
    event_type    VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100),
    resource_id   UUID,
    old_value     JSONB,
    new_value     JSONB,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
REVOKE UPDATE, DELETE ON audit_log FROM PUBLIC;
CREATE INDEX idx_audit_log_org ON audit_log(org_id, created_at DESC);

CREATE TABLE prompt_versions (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id      UUID         NOT NULL REFERENCES prompt_repositories(id),
    prompt_name  VARCHAR(255) NOT NULL,
    version      INTEGER      NOT NULL,
    content      TEXT         NOT NULL,
    config       JSONB        NOT NULL DEFAULT '{}',
    created_by   UUID         REFERENCES users(id),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (repo_id, prompt_name, version)
);

CREATE TABLE skills (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID         NOT NULL REFERENCES organizations(id),
    name        VARCHAR(255) NOT NULL,
    description VARCHAR(200) NOT NULL,
    version     INTEGER      NOT NULL DEFAULT 1,
    content     TEXT         NOT NULL,
    frontmatter JSONB        NOT NULL DEFAULT '{}',
    preload     BOOLEAN      NOT NULL DEFAULT FALSE,
    UNIQUE (org_id, name, version)
);

CREATE TABLE mcp_servers (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID         NOT NULL REFERENCES organizations(id),
    name            VARCHAR(255) NOT NULL,
    url             VARCHAR(512) NOT NULL,
    auth_type       VARCHAR(50)  NOT NULL,
    credentials_enc TEXT,
    tools           JSONB        NOT NULL DEFAULT '[]',
    status          VARCHAR(20)  NOT NULL DEFAULT 'active',
    UNIQUE (org_id, name)
);
```

### 10.5 Data Retention Policy

| Data Type        | Retention    | Action After Retention                      |
|------------------|--------------|---------------------------------------------|
| request_logs     | 90 days      | Archive to MinIO/S3; DROP old partition     |
| cost_records     | 2 years      | Archive to cold storage                     |
| audit_log        | 7 years      | Archive; never delete from cold storage     |
| auth_events      | 1 year       | Archive to cold storage                     |
| agent_sessions   | 24 hours TTL | DELETE WHERE ttl_expires_at < NOW()         |
| prompt_versions  | Indefinite   | Immutable; never deleted                    |
| provider creds   | Until deleted | AES-256 encrypted; purged on acct deletion |

---

## 11. Security Architecture

| ID      | Category          | Requirement                                          | Implementation                                         |
|---------|-------------------|------------------------------------------------------|--------------------------------------------------------|
| SEC-001 | Transport         | TLS 1.3 for all HTTP in production                   | Go net/http TLS config; HSTS header enforced           |
| SEC-002 | Transport         | Redis TLS in production                              | go-redis TLSConfig; skip in dev                        |
| SEC-003 | Secrets           | Provider API keys AES-256-GCM encrypted at rest      | Encrypt on write to DB; decrypt in memory only         |
| SEC-004 | Secrets           | No secrets in source code or Docker images           | All config via env vars; Gitleaks scan in CI           |
| SEC-005 | Auth              | Daemon-to-server authentication via API key          | Bearer token in all POST headers; key in DAEMON_API_KEY|
| SEC-006 | Auth              | User tokens: PAT / VAT / JWT RS256                   | Validated on every inquiry; 15-min JWT expiry          |
| SEC-007 | Audit             | Immutable audit log                                  | audit_log INSERT-only; REVOKE UPDATE/DELETE            |
| SEC-008 | Input Validation  | All incoming requests validated before processing    | Go struct validation; reject unknown fields            |
| SEC-009 | SQL Injection     | Parameterised queries throughout                     | pgx named parameters; no string concatenation in SQL  |
| SEC-010 | Dependency        | Known CVE scanning in CI                             | govulncheck ./... in GitHub Actions                    |

---

## 12. Deployment Architecture

### 12.1 Go Project Structure

```
pangreksa-gateway/
├── cmd/
│   ├── daemon/main.go              ← Daemon entry point
│   └── server/main.go              ← Central Server entry point
├── internal/
│   ├── probe/                      ← Liveness probe + backoff
│   ├── poller/                     ← Config poller goroutine
│   ├── cache/
│   │   ├── local/                  ← sync.Map + TTL eviction
│   │   ├── redis/                  ← Redis layer (go-redis/v9)
│   │   └── invalidation/           ← Redis SUBSCRIBE handler
│   ├── entitlement/                ← Two-layer resolver
│   ├── proxy/
│   │   ├── openai.go               ← OpenAI adapter
│   │   ├── claude.go               ← Anthropic adapter
│   │   ├── moonshot.go             ← Moonshot adapter
│   │   └── ollama.go               ← Ollama adapter
│   ├── registry/
│   │   ├── prompt/                 ← FQN resolve, template, guardrail union
│   │   ├── skill/                  ← Progressive disclosure
│   │   └── mcp/                    ← Tool whitelist, Cedar/OPA, HITL
│   ├── policy/
│   │   ├── guardrail/              ← 4 hooks, validate/mutate/block
│   │   ├── budget/                 ← YAML rules, Redis INCRBYFLOAT
│   │   └── ratelimit/              ← Sliding window, Lua script
│   ├── llm/                        ← Provider dispatch, circuit breaker
│   ├── producer/                   ← Kafka fire-and-forget producer
│   └── server/
│       ├── api/                    ← HTTP handlers (/health /config /entitlement)
│       ├── consumer/               ← Kafka consumer group
│       ├── configstore/            ← Build + serve GatewayConfig
│       ├── entitlement/            ← DB read + Redis PUBLISH
│       ├── repository/             ← pgx/v5 DB access layer
│       └── publisher/              ← Redis PUBLISH wrapper
├── pkg/
│   ├── model/
│   │   ├── config.go               ← GatewayConfig struct
│   │   ├── entitlement.go          ← UserEntitlement struct
│   │   ├── inquiry.go              ← InquiryRequest, TransactionEvent
│   │   └── policy.go               ← BudgetRule, RateLimitRule, GuardrailConfig
│   ├── logger/                     ← zerolog/zap structured logger
│   └── crypto/                     ← AES-256-GCM encrypt/decrypt helpers
├── migrations/
│   ├── 000001_init_schema.up.sql
│   ├── 000001_init_schema.down.sql
│   └── ...
├── configs/
│   ├── daemon.env.example
│   └── server.env.example
├── docker/
│   ├── Dockerfile.daemon           ← Multi-stage Go build
│   ├── Dockerfile.server
│   ├── docker-compose.infra.yml    ← PostgreSQL + Redis + Kafka
│   └── docker-compose.app.yml      ← Daemon + Server
├── go.work                         ← Go workspace
├── go.mod
├── Makefile
└── README.md
```

### 12.2 Environment Variables

```
═══ DAEMON ══════════════════════════════════════════════════════
  GATEWAY_ID=gw-prod-001
  CONFIG_SERVER_URL=http://central-server:9000
  DAEMON_API_KEY=<secret>
  REDIS_URL=redis://redis:6379
  KAFKA_BROKERS=kafka:9092
  POLL_INTERVAL_SEC=30
  MAX_LIVENESS_RETRIES=0     (0=infinite)
  ENCRYPT_KEY=<32-byte hex>  (AES-256 key)
  LOG_LEVEL=info
  OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317

═══ CENTRAL SERVER ══════════════════════════════════════════════
  DATABASE_URL=postgres://user:pass@postgres:5432/pangreksa?sslmode=disable
  REDIS_URL=redis://redis:6379
  KAFKA_BROKERS=kafka:9092
  SERVER_PORT=9000
  ADMIN_API_KEY=<secret>
  DAEMON_API_KEY=<secret>
  ENCRYPT_KEY=<32-byte hex>
  LOG_LEVEL=info
  OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317
```

### 12.3 Dockerfile (Multi-Stage)

```dockerfile
# Dockerfile.daemon
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o daemon ./cmd/daemon

FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/daemon /daemon
EXPOSE 8080 8081 8082 8083
ENTRYPOINT ["/daemon"]
```

### 12.4 CI/CD Pipeline

```
[Git Push to main]
    │
    ▼
[GitHub Actions]
    ├── go vet ./...
    ├── golangci-lint run
    ├── go test ./... -race -coverprofile=coverage.out
    ├── go tool covdata percent -i=coverage.out  (>= 80% check)
    ├── govulncheck ./...
    ├── docker build -f docker/Dockerfile.daemon
    ├── docker build -f docker/Dockerfile.server
    ├── docker push to registry (tagged with git SHA)
    ├── deploy to staging (docker compose pull && up -d)
    ├── integration tests against staging
    └── (manual approval) → deploy to production
```

### 12.5 Environment Strategy

| Environment | Infrastructure                        | Scale                        |
|-------------|---------------------------------------|------------------------------|
| Development | go run; Docker Compose for infra      | Single process per component |
| Windows Dev | go run natively; local PG/Redis/Kafka | Single process               |
| Staging     | Full Docker Compose stack             | 1 daemon + 1 server          |
| Production  | Docker Swarm or Kubernetes            | N daemon + M server replicas |

---

## 13. Risks & Mitigation

| ID      | Risk                                             | Likelihood | Impact | Mitigation                                                        |
|---------|--------------------------------------------------|------------|--------|-------------------------------------------------------------------|
| RSK-001 | Config poll delay causes slow propagation        | Medium     | Medium | Tune POLL_INTERVAL_SEC; /admin/invalidate for forced refresh      |
| RSK-002 | Kafka lag delays transaction log persistence     | Low        | Medium | Consumer lag monitoring; budget uses Redis (not Kafka)            |
| RSK-003 | Redis down — rate/budget counters unavailable    | Low        | High   | Circuit breaker; fallback to allow with audit mode logging        |
| RSK-004 | sync.Map stale on daemon crash + restart         | Low        | Medium | TTL enforced; cold start re-fetches from Redis then server        |
| RSK-005 | PostgreSQL bottleneck on request_log inserts     | Medium     | Medium | Monthly partitions; PgBouncer; batch inserts in Kafka consumer    |
| RSK-006 | Windows filepath issues in Go build              | Low        | Medium | filepath.Join throughout; CI tests on windows-latest runner       |
| RSK-007 | AES key rotation breaks existing encrypted creds | Low        | High   | Key versioning prefix in encrypted fields; rotation runbook       |
| RSK-008 | MCP HITL timeout causes stuck agent sessions     | Medium     | Medium | 5-min configurable timeout; session TTL cleanup job               |
| RSK-009 | Guardrail provider down blocks all requests      | Medium     | High   | Enforce-But-Degrade mode per guardrail; circuit breaker           |
| RSK-010 | Thundering herd on daemon restart (all cold)     | Medium     | Medium | Jittered liveness backoff (2s initial); stagger pod restarts      |

---

## 14. Appendix

### 14.1 Glossary

| Term               | Definition                                                                    |
|--------------------|-------------------------------------------------------------------------------|
| chan Config        | Go buffered channel (size 1) carrying GatewayConfig between goroutines        |
| sync.Map           | Go built-in concurrent-safe map; Layer 1 of entitlement cache (~50ns)         |
| Fire and forget    | Kafka publish that does not block caller; failures retried in background      |
| Sliding window     | Rate limit algorithm: 5s time buckets, 12-bucket lookback (60s window)        |
| Progressive disclosure | Sending only skill name+description upfront; full body on LLM selection  |
| HITL               | Human-in-the-Loop: pauses agent for human approval before destructive tool    |
| FQN                | Fully Qualified Name: chat_prompt:{repo}/{name}:{version}                     |
| Cold path          | Startup: liveness probe -> config poll -> invalidation subscribe              |
| Hot path           | Per-inquiry: proxy -> entitlement -> registry -> policy -> LLM -> Kafka       |
| Two-layer cache    | sync.Map (L1) + Redis (L2) + Central Server (L3) for entitlement resolution   |

### 14.2 Key Go Dependencies

| Package                                    | Purpose                                  |
|--------------------------------------------|------------------------------------------|
| github.com/redis/go-redis/v9               | Redis client (cluster + pub/sub + Lua)   |
| github.com/confluentinc/confluent-kafka-go | Kafka producer + consumer                |
| github.com/jackc/pgx/v5                    | PostgreSQL driver + connection pool      |
| github.com/golang-migrate/migrate/v4       | Database migrations                      |
| github.com/rs/zerolog                      | Structured JSON logging                  |
| go.opentelemetry.io/otel                   | Distributed tracing (OTLP export)        |
| github.com/open-policy-agent/opa           | OPA Rego policy evaluation (MCP guard)   |
| golang.org/x/crypto                        | AES-256-GCM, Argon2id                    |
| github.com/stretchr/testify                | Unit + integration test assertions       |

### 14.3 References

| #    | Reference                                                                    |
|------|------------------------------------------------------------------------------|
| R-01 | SRS-PANGREKSA-AIROUTERGATEWAY-001 v1.0 (parent SRS)                         |
| R-02 | iSAQB CPSA-A Curriculum — https://www.isaqb.org                             |
| R-03 | ISO/IEC 29148:2018 — Requirements Engineering                                |
| R-04 | Go 1.22 Release Notes — https://go.dev/doc/go1.22                           |
| R-05 | golang-migrate — https://github.com/golang-migrate/migrate                  |
| R-06 | pgx v5 — https://github.com/jackc/pgx                                       |
| R-07 | go-redis v9 — https://github.com/redis/go-redis                             |
| R-08 | confluent-kafka-go — https://github.com/confluentinc/confluent-kafka-go     |
| R-09 | OpenTelemetry Go — https://opentelemetry.io/docs/languages/go               |
| R-10 | govulncheck — https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck          |

---

*End of SRS — Pangreksa AI Gateway Engine v1.0*

---

> | Version | Date       | Author                        | Change          |
> |---------|------------|-------------------------------|-----------------|
> | 1.0     | 2026-06-04 | AI Software Architect (iSAQB) | Initial draft   |
