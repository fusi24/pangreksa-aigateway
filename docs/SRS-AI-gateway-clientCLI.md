# Software Requirements Specification
# Pangreksa AI Gateway Client CLI

---

| Field         | Value                                                          |
|---------------|----------------------------------------------------------------|
| Document ID   | SRS-AI-GATEWAY-CLIENT-CLI-001                                  |
| Version       | 1.0                                                            |
| Status        | Draft                                                          |
| Prepared by   | AI Software Architect (iSAQB CPSA-A Aligned)                  |
| Date          | 2026-06-04                                                     |
| Standard      | iSAQB CPSA-A / ISO/IEC 29148:2018                             |
| Technology    | Go 1.22+, OpenAI-compatible API, Windows/Linux                 |

---

## Table of Contents

1. Introduction
2. System Overview
3. Architecture
4. Functional Requirements
5. Configuration Reference
6. API Contract
7. Security Architecture
8. Non-Functional Requirements
9. Deployment Architecture
10. Example Sessions
11. Risks & Mitigation
14. Appendix

---

## 1. Introduction

### 1.1 Purpose

This Software Requirements Specification (SRS) defines the complete functional and
non-functional requirements for the **Pangreksa AI Gateway Client CLI** — a lightweight,
interactive and scriptable command-line test engine for the Gateway Daemon. This document
targets: Go developers, QA engineers, DevOps engineers, and software architects working on
the Pangreksa AI Gateway Engine.

The Client CLI serves three primary purposes:
- **Interactive REPL mode** — a multi-turn conversational session against the Gateway Daemon
- **Single-shot mode** — a one-line query for scripting and automation pipelines
- **Stdin-pipe mode** — reads a prompt from stdin when running in a non-interactive context

### 1.2 Scope

**System Name:** Pangreksa AI Gateway Client CLI

**In Scope:**
- Interactive REPL mode with in-memory multi-turn conversation history
- Single-shot mode via `-message` flag
- Stdin-pipe mode when stdin is not a TTY
- Non-streaming response (default) and streaming response via SSE (`-stream` flag)
- Bearer token authentication against the Gateway Daemon
- Configurable daemon URL, model, and system prompt via flags and environment variables
- Clear error display with non-zero exit code on failure
- Single-binary delivery; no external dependencies beyond Go stdlib

**Out of Scope:**
- Console or web UI (covered by a separate system)
- Authentication server or token issuance
- Direct communication with LLM providers (all requests go through the Gateway Daemon)
- Configuration management of the Gateway Daemon itself
- Guardrail, budget, or rate-limit configuration

### 1.3 Definitions & Acronyms

| Term          | Definition                                                                 |
|---------------|----------------------------------------------------------------------------|
| SRS           | Software Requirements Specification                                        |
| CLI           | Command-Line Interface                                                     |
| REPL          | Read-Eval-Print Loop — interactive session that reads input, sends it to  |
|               | the daemon, prints the response, and loops until the user exits            |
| SSE           | Server-Sent Events — HTTP streaming protocol where the server pushes      |
|               | `data: {...}` lines over a persistent connection                           |
| Bearer token  | HTTP Authorization header value: `Authorization: Bearer <token>`          |
| Single-shot   | One-request-one-response mode; the CLI exits after printing the answer    |
| Stdin-pipe    | Non-interactive mode; prompt is read from stdin (e.g. `echo "hi" | cli`) |
| TTY           | Teletypewriter — a terminal device; `os.Stdin` is a TTY in interactive    |
|               | shells and not a TTY when piped                                            |
| ADR           | Architecture Decision Record                                               |
| NFR           | Non-Functional Requirement                                                 |

### 1.4 References

| #    | Reference                                                                   |
|------|-----------------------------------------------------------------------------|
| R-01 | SRS-AI-GATEWAY-ENGINE-001 v1.0 — Pangreksa AI Gateway Engine               |
| R-02 | OpenAI Chat Completions API — https://platform.openai.com/docs/api-reference/chat |
| R-03 | iSAQB CPSA-A Curriculum — https://www.isaqb.org                            |
| R-04 | ISO/IEC 29148:2018 — Requirements Engineering                               |
| R-05 | Go 1.22+ Documentation — https://go.dev/doc                                |
| R-06 | Server-Sent Events — https://html.spec.whatwg.org/multipage/server-sent-events.html |

---

## 2. System Overview

### 2.1 System Context

```
╔══════════════════════════════════════════════════════════════════════════╗
║                         SYSTEM BOUNDARY                                  ║
║                                                                          ║
║   ┌─────────────────┐          ┌──────────────────────────────────┐      ║
║   │   USER          │          │      CLIENT CLI                  │      ║
║   │                 │  stdin / │      (Go, single binary)         │      ║
║   │  Interactive    │  flag    │                                  │      ║
║   │  Terminal       │─────────►│  Config Loader                   │      ║
║   │  Shell Script   │          │  Request Builder                 │      ║
║   │  CI Pipeline    │◄─────────│  HTTP Client (stdlib net/http)   │      ║
║   └─────────────────┘  stdout  │  Response Parser                 │      ║
║                                │  Output Formatter                │      ║
║                                └──────────────┬───────────────────┘      ║
║                                               │                          ║
║                              Bearer token     │  POST /v1/chat/completions║
║                              HTTP/HTTPS       │                          ║
║                                               ▼                          ║
║                                ┌──────────────────────────────────┐      ║
║                                │     GATEWAY DAEMON :8080         │      ║
║                                │     (Go, Docker)                 │      ║
║                                │                                  │      ║
║                                │  Proxy → Registry → Policy       │      ║
║                                │       → LLM dispatch             │      ║
║                                └──────────────────────────────────┘      ║
║                                               │                          ║
╚═══════════════════════════════════════════════╪══════════════════════════╝
                                                │
                                                ▼
                                  ┌──────────────────────────┐
                                  │      LLM PROVIDER        │
                                  │  OpenAI · Anthropic      │
                                  │  Moonshot · Ollama       │
                                  └──────────────────────────┘
```

The Client CLI is a **pure HTTP client**. It holds no server socket, no background goroutines
(except the SSE stream reader), and no persistent state between runs. The REPL holds
conversation history only in process memory for the lifetime of the session.

### 2.2 System Goals

| ID           | Goal                                                                          |
|--------------|-------------------------------------------------------------------------------|
| GOAL-CLI-001 | Test the Gateway Daemon end-to-end without external tooling                   |
| GOAL-CLI-002 | Support interactive multi-turn conversations via REPL mode                    |
| GOAL-CLI-003 | Support scripting via single-shot and stdin-pipe modes                        |
| GOAL-CLI-004 | Display streaming tokens in real time via SSE                                 |
| GOAL-CLI-005 | Zero external Go dependencies for easy audit and Windows builds               |

### 2.3 Constraints

| Type        | Constraint                                                                    |
|-------------|-------------------------------------------------------------------------------|
| Language    | Go 1.22+ — standard library only for HTTP and I/O                            |
| HTTP client | stdlib `net/http` — no external HTTP client libraries                        |
| Platform    | Windows amd64 (primary); Linux amd64 (secondary)                             |
| Binary      | Single compiled binary; `go build ./cmd/client`                              |
| Secrets     | No secrets hardcoded in source; `DAEMON_TOKEN` from env var or `-token` flag |
| CGO         | `CGO_ENABLED=0` — pure Go build, no C dependencies                           |
| API target  | OpenAI-compatible Chat Completions endpoint on the Gateway Daemon             |

---

## 3. Architecture

### 3.1 Architectural Style

**Single-Process CLI Tool — Layered + Stateless**

- **CLI tool**: No server component, no background goroutines at rest
- **Stateless between runs**: All state (history, token, config) lives only in memory for
  one process lifetime; the REPL holds conversation history in a `[]Message` slice
- **Layered pipeline**: Each component has a single responsibility and passes data to the next
- **Flag-then-env precedence**: Flags take precedence over environment variables for all config

### 3.2 Component Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                        CLIENT CLI PROCESS                           │
│                                                                     │
│  ┌──────────────────┐                                               │
│  │  CONFIG LOADER   │  Reads flags → env vars → defaults           │
│  │  (flag + os.Env) │  Validates required fields (DAEMON_URL,      │
│  └────────┬─────────┘  DAEMON_TOKEN)                               │
│           │                                                         │
│           ▼                                                         │
│  ┌──────────────────┐                                               │
│  │  MODE SELECTOR   │  Detects which mode to run:                  │
│  │                  │   -message flag set  → Single-shot           │
│  │                  │   stdin not a TTY    → Stdin-pipe            │
│  │                  │   otherwise          → REPL                  │
│  └────────┬─────────┘                                               │
│           │                                                         │
│           ▼                                                         │
│  ┌──────────────────┐                                               │
│  │  REQUEST BUILDER │  Constructs ChatCompletionRequest:           │
│  │                  │   - model, stream flag                       │
│  │                  │   - system message (if configured)           │
│  │                  │   - conversation history (REPL only)         │
│  │                  │   - current user message                     │
│  └────────┬─────────┘                                               │
│           │                                                         │
│           ▼                                                         │
│  ┌──────────────────┐                                               │
│  │  HTTP CLIENT     │  stdlib net/http                             │
│  │  (net/http)      │   POST {DAEMON_URL}/v1/chat/completions      │
│  │                  │   Authorization: Bearer {DAEMON_TOKEN}       │
│  │                  │   Content-Type: application/json             │
│  └────────┬─────────┘                                               │
│           │                                                         │
│           ▼                                                         │
│  ┌──────────────────┐                                               │
│  │  RESPONSE PARSER │  Non-stream: JSON unmarshal                  │
│  │                  │  Stream: SSE line-by-line reader             │
│  │                  │   - parse `data: {...}` lines                │
│  │                  │   - detect `data: [DONE]` terminator         │
│  └────────┬─────────┘                                               │
│           │                                                         │
│           ▼                                                         │
│  ┌──────────────────┐                                               │
│  │  OUTPUT FORMATTER│  Prints assistant content to stdout          │
│  │                  │  Stream: prints tokens as they arrive        │
│  │                  │  Errors: prints to stderr, exit 1            │
│  └──────────────────┘                                               │
│                                                                     │
│  REPL LOOP (Interactive mode only):                                 │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  Print "> " → Read line from stdin                          │    │
│  │  if "exit" or "quit" → print "Goodbye." → os.Exit(0)       │    │
│  │  Append to history → Build request → Call HTTP Client       │    │
│  │  Append assistant response to history → loop               │    │
│  └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
```

### 3.3 Architecture Decision Records (ADR)

#### ADR-CLI-001: stdlib net/http (No External SDK)
- **Status:** Accepted
- **Context:** The CLI must be a single binary with zero external HTTP client dependencies.
  The Gateway Daemon exposes an OpenAI-compatible REST API — a well-specified JSON/HTTP
  interface requiring no SDK-specific abstractions.
- **Decision:** Use Go stdlib `net/http` for all HTTP requests. Use `encoding/json` for
  serialisation. Use `bufio.Scanner` for SSE stream line reading.
- **Rationale:** Zero dependencies; compiles on Windows/Linux natively; easy to audit;
  no version conflicts; `go.mod` remains unchanged from the engine module.
- **Consequences:** SSE streaming must be implemented manually (line-by-line reader),
  which is straightforward given the simple `data: {...}` / `data: [DONE]` format.

#### ADR-CLI-002: Flags + Env Vars (Flags Take Precedence)
- **Status:** Accepted
- **Context:** The CLI must work in two distinct contexts:
  1. Interactive developer use — flags are convenient and explicit
  2. CI/CD pipelines and scripts — environment variables keep command lines clean and
     avoid token exposure in shell history
- **Decision:** All configuration has both a `-flag` form and a `DAEMON_*` env var form.
  Flag values always win over env vars. Env vars win over compiled defaults.
- **Rationale:** Scripting-friendly (env vars in CI) while keeping interactive UX clean
  (flags for ad-hoc overrides). Follows the precedence convention of `docker`, `kubectl`,
  and other well-known Go CLI tools.
- **Consequences:** Config loader must check flags first, then `os.Getenv`, then defaults.
  This is implemented in the `loadConfig()` function before any mode is entered.

---

## 4. Functional Requirements

### 4.1 Mode Detection

```
CLI STARTUP — MODE SELECTION
══════════════════════════════

main()
  │
  ├──[1] loadConfig()  (flags → env → defaults)
  │       Validate DAEMON_URL set; DAEMON_TOKEN set
  │       On validation error: print to stderr, os.Exit(1)
  │
  ├──[2] Detect mode:
  │
  │       -message flag provided?
  │          YES ──► Single-shot mode (SRS-FR-CLI-002)
  │          NO  ──►
  │
  │       isatty(os.Stdin)?
  │          NO  ──► Stdin-pipe mode (SRS-FR-CLI-003)
  │          YES ──► REPL mode (SRS-FR-CLI-001)
  │
  └──[3] Run selected mode
```

---

**SRS-FR-CLI-001: Interactive REPL Mode**

| Field            | Value                                                                      |
|------------------|----------------------------------------------------------------------------|
| Priority         | Critical                                                                   |
| Component        | cmd/client/repl.go                                                         |
| Description      | When stdin is a TTY and `-message` is not set, run an interactive loop     |

```
REPL MODE FLOW
═══════════════

START
  │
  ├─ Print welcome + config summary
  │
  └─ LOOP:
       Print "> "
       Read line
         │
         ├─ "exit"/"quit" → EXIT 0
         │
         └─ Send POST /v1/chat/completions
              │
              ├─ Error → print error, continue loop
              │
              └─ OK → append to history, print response, continue loop
```

**Acceptance Criteria:**

| Given                                    | When                          | Then                                              |
|------------------------------------------|-------------------------------|---------------------------------------------------|
| REPL is running                          | User types a message          | Request sent; assistant reply printed             |
| REPL is running                          | User types "exit"             | Loop terminates; "Goodbye." printed; exit 0       |
| REPL is running                          | HTTP 429 returned by daemon   | Error printed to stderr; loop continues           |
| REPL has prior messages in history       | User sends next message       | Full history included in request body             |
| REPL is running                          | User presses Ctrl-D (EOF)     | Loop terminates cleanly; exit 0                   |

---

**SRS-FR-CLI-002: Single-Shot Mode**

| Field            | Value                                                                      |
|------------------|----------------------------------------------------------------------------|
| Priority         | Critical                                                                   |
| Component        | cmd/client/main.go                                                         |
| Description      | When `-message` flag is provided, send one request, print response, exit  |

```
SINGLE-SHOT MODE FLOW
══════════════════════

-message "Translate 'hello' to Japanese"
    │
    ▼
Build ChatCompletionRequest
  messages: [
    {role:"system", content: SYSTEM_PROMPT}  (if set)
    {role:"user",   content: flag.message}
  ]
    │
    ▼
POST /v1/chat/completions
    │
    ├── On error ──► print error to stderr, os.Exit(1)
    │
    └── On success ──► print assistant content to stdout, os.Exit(0)
```

**Acceptance Criteria:**

| Given                          | When                                         | Then                                   |
|--------------------------------|----------------------------------------------|----------------------------------------|
| `-message "Hello"` is set      | CLI runs                                     | Response printed; exit 0              |
| `-message` set + daemon down   | CLI runs                                     | Error printed to stderr; exit 1       |
| `-message` set + HTTP 401      | CLI runs                                     | "Unauthorized" error; exit 1          |

---

**SRS-FR-CLI-003: Stdin-Pipe Mode**

| Field            | Value                                                                      |
|------------------|----------------------------------------------------------------------------|
| Priority         | High                                                                       |
| Component        | cmd/client/main.go                                                         |
| Description      | When stdin is not a TTY (pipe or redirect), read full stdin as the prompt |

```
STDIN-PIPE MODE FLOW
═════════════════════

Detected: os.Stdin is not a TTY (e.g. echo "prompt" | ./client)
    │
    ▼
Read all of stdin (io.ReadAll) as promptText
    │
Empty stdin ──► print error to stderr, os.Exit(1)
    │
    ▼
Build ChatCompletionRequest (same as single-shot, using promptText)
    │
    ▼
POST /v1/chat/completions → print response → os.Exit(0 or 1)
```

**Acceptance Criteria:**

| Given                                        | When                         | Then                              |
|----------------------------------------------|------------------------------|-----------------------------------|
| `echo "Hello" \| ./client`                  | CLI runs                     | Response printed; exit 0         |
| `cat prompt.txt \| ./client`                | CLI runs                     | File contents sent as prompt     |
| Empty stdin (e.g. `echo "" \| ./client`)    | CLI runs                     | Error "empty input"; exit 1      |

---

**SRS-FR-CLI-004: Non-Streaming Response (Default)**

| Field            | Value                                                                      |
|------------------|----------------------------------------------------------------------------|
| Priority         | High                                                                       |
| Component        | cmd/client/http.go                                                         |
| Description      | Default mode: send `"stream": false`; wait for full JSON response         |

```
NON-STREAMING RESPONSE HANDLING
═════════════════════════════════

POST /v1/chat/completions
  Body: { ..., "stream": false }
    │
    ▼
Read full HTTP response body (io.ReadAll)
    │
    ▼
json.Unmarshal into ChatCompletionResponse
    │
Parse: choices[0].message.content
    │
    ▼
Print content to stdout
```

---

**SRS-FR-CLI-005: Streaming Response via SSE**

| Field            | Value                                                                      |
|------------------|----------------------------------------------------------------------------|
| Priority         | Medium                                                                     |
| Component        | cmd/client/stream.go                                                       |
| Description      | When `-stream` flag is set, send `"stream": true`; read SSE lines         |

```
STREAMING (SSE) RESPONSE HANDLING
════════════════════════════════════

POST /v1/chat/completions
  Body: { ..., "stream": true }
    │
    ▼
HTTP response: 200 OK
  Content-Type: text/event-stream
    │
    ▼
bufio.Scanner reading response body line by line:

  For each line:
  ┌─────────────────────────────────────────────────────┐
  │  line == ""                ──► skip (blank line)    │
  │  line == "data: [DONE]"    ──► break (stream ended) │
  │  line starts with "data: " ──►                      │
  │    strip "data: " prefix                            │
  │    json.Unmarshal into ChatCompletionChunk          │
  │    extract choices[0].delta.content                 │
  │    fmt.Print(content)  (no newline — stream tokens) │
  └─────────────────────────────────────────────────────┘
    │
    ▼
fmt.Println()  (final newline after stream ends)
```

**Acceptance Criteria:**

| Given                       | When                        | Then                                           |
|-----------------------------|-----------------------------|------------------------------------------------|
| `-stream` flag set          | Daemon returns SSE stream   | Tokens printed to stdout as they arrive        |
| `-stream` flag set          | `data: [DONE]` received     | Stream ends; newline printed; exit 0          |
| `-stream` flag set          | HTTP 500 returned           | Error printed to stderr; exit 1               |

---

**SRS-FR-CLI-006: Bearer Token Authentication**

| Field            | Value                                                                      |
|------------------|----------------------------------------------------------------------------|
| Priority         | Critical                                                                   |
| Component        | cmd/client/http.go                                                         |
| Description      | Every HTTP request MUST include `Authorization: Bearer {DAEMON_TOKEN}`    |

```
AUTHENTICATION FLOW
════════════════════

Every outbound request:
  req.Header.Set("Authorization", "Bearer " + config.Token)
  req.Header.Set("Content-Type", "application/json")

Token source (in precedence order):
  1. -token flag
  2. DAEMON_TOKEN env var
  3. (none) ──► print error "DAEMON_TOKEN is required"; os.Exit(1)

Token in logs:
  NEVER logged in full
  If debug logging enabled: log "Bearer ***" (masked)
```

---

**SRS-FR-CLI-007: Configurable Daemon URL, Model, System Prompt**

| Field            | Value                                                                      |
|------------------|----------------------------------------------------------------------------|
| Priority         | High                                                                       |
| Component        | cmd/client/config.go                                                       |
| Description      | URL, model, and system prompt are all configurable via flag or env var    |

```
CONFIG LOAD ORDER (per field)
══════════════════════════════

For each config field:
  1. Check flag value (flag.Parse() result)
  2. If flag not set: check os.Getenv(DAEMON_*)
  3. If env not set: use default

Required fields (no default):
  DAEMON_URL   / -url    ──► os.Exit(1) if missing
  DAEMON_TOKEN / -token  ──► os.Exit(1) if missing
```

---

**SRS-FR-CLI-008: Clear Error Display with Non-Zero Exit on Failure**

| Field            | Value                                                                      |
|------------------|----------------------------------------------------------------------------|
| Priority         | High                                                                       |
| Component        | cmd/client/error.go                                                        |
| Description      | All fatal errors print to stderr and exit with code 1                     |

```
ERROR HANDLING MATRIX
══════════════════════

HTTP Status    Exit Code    Stderr Output
───────────    ─────────    ─────────────────────────────────────────
401            1            "Error: unauthorized — check DAEMON_TOKEN"
403            1            "Error: forbidden — insufficient permissions"
404            1            "Error: not found — check DAEMON_URL and model"
429            see below    "Error: rate limited — retry after {X}s"
5xx            1            "Error: server error ({status}) — {body}"
Network err    1            "Error: could not reach daemon — {err}"

Special case — REPL mode:
  HTTP errors (including 429) print to stderr but DO NOT exit the loop.
  The REPL continues and the user can send the next message.

Single-shot and Stdin-pipe modes:
  Any error causes os.Exit(1).
```

---

**SRS-FR-CLI-009: Multi-Turn Conversation History in REPL**

| Field            | Value                                                                      |
|------------------|----------------------------------------------------------------------------|
| Priority         | Medium                                                                     |
| Component        | cmd/client/repl.go                                                         |
| Description      | REPL accumulates conversation history in memory; sent with every request  |

```
CONVERSATION HISTORY MANAGEMENT
═════════════════════════════════

history := []Message{}

On system prompt configured:
  history = append(history, Message{Role: "system", Content: systemPrompt})

Per turn:
  history = append(history, Message{Role: "user",      Content: userInput})
  // POST with full history
  history = append(history, Message{Role: "assistant", Content: assistantReply})

Constraints:
  - History lives only in process memory (not persisted to disk)
  - No automatic truncation — full history sent on every request
  - History is lost when the REPL session ends
```

---

**SRS-FR-CLI-010: exit/quit Command in REPL**

| Field            | Value                                                                      |
|------------------|----------------------------------------------------------------------------|
| Priority         | Low                                                                        |
| Component        | cmd/client/repl.go                                                         |
| Description      | Typing `exit` or `quit` (case-insensitive) terminates the REPL cleanly   |

```
EXIT COMMAND HANDLING
══════════════════════

User input (trimmed, lowercased):
  "exit" OR "quit" ──► print "Goodbye." ──► os.Exit(0)
  Ctrl-D (EOF)     ──► print "Goodbye." ──► os.Exit(0)
  Ctrl-C (SIGINT)  ──► Go default signal handler ──► os.Exit(2)
```

---

## 5. Configuration Reference

All configuration options are resolved in this order: **command-line flag → environment variable → built-in default**.

| Flag       | Env Var         | Default                    | Required | Description                                         |
|------------|-----------------|----------------------------|----------|-----------------------------------------------------|
| `-url`     | `DAEMON_URL`    | `http://localhost:8080`    | No       | Gateway Daemon base URL (scheme + host + port)      |
| `-model`   | `DAEMON_MODEL`  | `gpt-4o`                   | No       | Model name passed to the daemon                     |
| `-token`   | `DAEMON_TOKEN`  | `dev-test-pat-token-hash`  | No       | Bearer authentication token                         |
| `-message` | —               | `""`                       | No       | Single-shot message text; triggers single-shot mode |
| `-system`  | `DAEMON_SYSTEM` | `""`                       | No       | System prompt prepended to every request            |
| `-stream`  | —               | `false`                    | No       | Enable SSE token streaming                          |

**Notes:**
- Flag values always take precedence over environment variables.
- `-message` flag has no env var equivalent — it is inherently a runtime argument.
- `-token` default is the development test token; it MUST be overridden in any non-local environment.
- `-stream` is a boolean flag; pass it with no value (e.g. `./client -stream`) or as `-stream=true`.
- Token is masked as `***` in all log/debug output; never printed in full.

---

## 6. API Contract

### 6.1 Request Format

The Client CLI posts to the OpenAI-compatible Chat Completions endpoint on the Gateway Daemon.

**Non-streaming request:**

```json
POST /v1/chat/completions
Authorization: Bearer <DAEMON_TOKEN>
Content-Type: application/json

{
  "model": "gpt-4o",
  "stream": false,
  "messages": [
    {
      "role": "system",
      "content": "You are a helpful assistant."
    },
    {
      "role": "user",
      "content": "What is the capital of France?"
    }
  ]
}
```

**Streaming request (`-stream` flag):**

```json
POST /v1/chat/completions
Authorization: Bearer <DAEMON_TOKEN>
Content-Type: application/json

{
  "model": "gpt-4o",
  "stream": true,
  "messages": [
    {
      "role": "user",
      "content": "Explain quantum entanglement briefly."
    }
  ]
}
```

**Multi-turn request (REPL after second message):**

```json
{
  "model": "gpt-4o",
  "stream": false,
  "messages": [
    { "role": "system",    "content": "You are a helpful assistant." },
    { "role": "user",      "content": "What is the capital of France?" },
    { "role": "assistant", "content": "The capital of France is Paris." },
    { "role": "user",      "content": "And of Germany?" }
  ]
}
```

### 6.2 Response Format (Non-Streaming)

```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1748990400,
  "model": "gpt-4o",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "The capital of France is Paris."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 20,
    "completion_tokens": 9,
    "total_tokens": 29
  }
}
```

The client extracts: `choices[0].message.content`

### 6.3 SSE Stream Format

When `"stream": true`, the daemon returns a streaming response in Server-Sent Events format.
Each chunk contains a partial token delta.

```
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1748990400,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"The"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1748990400,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":" capital"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1748990400,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":" of France is Paris."},"finish_reason":"stop"}]}

data: [DONE]
```

**Parsing rules:**
- Skip blank lines
- Skip lines not starting with `data: `
- Stop when line is exactly `data: [DONE]`
- Extract `choices[0].delta.content` from each valid `data:` line
- Print each delta immediately (no buffering) for real-time display

### 6.4 Error Response Format

The Gateway Daemon returns errors in the following format. The client MUST handle all
listed HTTP status codes.

```json
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{
  "error": "unauthorized"
}
```

| HTTP Status | Meaning                                | Client Behaviour                             |
|-------------|----------------------------------------|----------------------------------------------|
| 401         | Bearer token invalid or missing        | Print "Error: unauthorized"; exit 1 (or continue REPL) |
| 403         | Token valid but insufficient permission| Print "Error: forbidden"; exit 1 (or continue REPL) |
| 404         | Model or endpoint not found            | Print "Error: not found"; exit 1 (or continue REPL) |
| 429         | Rate limited or budget exceeded        | Print "Error: rate limited — retry after Xs"; continue REPL or exit 1 |
| 500–599     | Server-side error                      | Print "Error: server error ({status})"; exit 1 (or continue REPL) |

---

## 7. Security Architecture

| ID          | Requirement                           | Implementation                                          |
|-------------|---------------------------------------|---------------------------------------------------------|
| SEC-CLI-001 | Bearer token never logged             | Token masked as `***` in all debug/log output; `fmt.Fprintf(os.Stderr, "Bearer ***")` pattern used |
| SEC-CLI-002 | No secrets in source code             | `DAEMON_TOKEN` sourced exclusively from env var or `-token` flag; no default value; no hardcoded fallback |
| SEC-CLI-003 | TLS support                           | `net/http` default transport respects `https://` in `DAEMON_URL`; no custom TLS config required for standard certs |
| SEC-CLI-004 | No token in shell history             | `DAEMON_TOKEN` env var usage is preferred over `-token` flag to avoid token appearing in shell history |
| SEC-CLI-005 | Request body never logged             | No request/response body written to any log file or stderr in non-debug builds |

---

## 8. Non-Functional Requirements

| ID          | Attribute       | Requirement                                              | Metric                                                     |
|-------------|-----------------|----------------------------------------------------------|------------------------------------------------------------|
| NFR-CLI-001 | Portability     | Windows amd64 native build                               | `GOOS=windows GOARCH=amd64 go build ./cmd/client` succeeds |
| NFR-CLI-002 | Portability     | No CGO                                                   | `CGO_ENABLED=0 go build ./cmd/client` succeeds             |
| NFR-CLI-003 | Performance     | Response display latency                                 | First token (stream) or full response printed within 2s on local network (excluding LLM provider latency) |
| NFR-CLI-004 | Usability       | Interactive mode startup                                 | REPL prompt visible within 100ms of process start          |
| NFR-CLI-005 | Maintainability | No external dependencies                                 | `go.mod` shows no new direct dependencies added by the client |
| NFR-CLI-006 | Reliability     | Non-fatal HTTP errors in REPL                            | HTTP errors print to stderr and keep the REPL loop running; the process does not exit |
| NFR-CLI-007 | Usability       | Meaningful error messages                                | Every error message includes the HTTP status code and a human-readable description |
| NFR-CLI-008 | Portability     | Linux amd64 build                                        | `GOOS=linux GOARCH=amd64 go build ./cmd/client` succeeds   |
| NFR-CLI-009 | Testability     | Unit-testable components                                 | Config loader, request builder, and response parser are pure functions with no global state |

---

## 9. Deployment Architecture

### 9.1 Run Directly

```bash
go run ./cmd/client
```

### 9.2 Build Single Binary

```bash
# Linux
go build -o bin/client ./cmd/client

# Windows
go build -o bin/client.exe ./cmd/client

# Cross-compile from Linux to Windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/client.exe ./cmd/client
```

### 9.3 PowerShell Helper

Reference `scripts/run-client.ps1` for a convenience wrapper that sets environment variables
and invokes the built binary.

```powershell
# scripts/run-client.ps1
$env:DAEMON_URL   = "http://localhost:8080"
$env:DAEMON_MODEL = "gpt-4o"
$env:DAEMON_TOKEN = "dev-test-pat-token-hash"
& "$PSScriptRoot\..\bin\client.exe" @args
```

### 9.4 Environment Variable Quick-Start

```
# Minimum required
DAEMON_URL=http://localhost:8080
DAEMON_TOKEN=<your-bearer-token>

# Optional
DAEMON_MODEL=gpt-4o
DAEMON_SYSTEM=You are a helpful assistant.
DAEMON_STREAM=false
DAEMON_TIMEOUT=60
```

### 9.5 .claude/launch.json Configuration

Add the following configuration block to `.claude/launch.json` alongside the existing
`Gateway Engine` and `Central Server Engine` configurations:

```json
{
  "name": "Client CLI",
  "type": "go",
  "request": "launch",
  "mode": "debug",
  "program": "${workspaceFolder}/cmd/client",
  "env": {
    "DAEMON_URL":   "http://localhost:8080",
    "DAEMON_MODEL": "gpt-4o",
    "DAEMON_TOKEN": "dev-test-pat-token-hash"
  },
  "console": "integratedTerminal"
}
```

### 9.6 Go Project Structure

```
pangreksa-gateway/
├── cmd/
│   ├── client/
│   │   ├── main.go         ← Entry point; flag parsing; mode selection
│   │   ├── config.go       ← Config struct; loadConfig() (flags → env → defaults)
│   │   ├── repl.go         ← REPL loop; history management
│   │   ├── http.go         ← HTTP client; request builder; non-stream reader
│   │   ├── stream.go       ← SSE stream reader; delta printer
│   │   └── error.go        ← Error formatting; exit code logic
│   ├── daemon/
│   └── server/
├── scripts/
│   ├── run-client.ps1      ← Windows PowerShell launcher
│   ├── run-daemon.ps1
│   └── run-server.ps1
└── ...
```

---

## 10. Example Sessions

### 10.1 Interactive Mode (REPL)

```
$ go run ./cmd/client
Pangreksa AI Gateway Client
  model : gpt-4o
  daemon: http://localhost:8080
Type 'exit' to quit.

> What is the capital of France?
Assistant: The capital of France is Paris.

> And of Germany?
Assistant: The capital of Germany is Berlin.

> exit
Goodbye.
```

### 10.2 Single-Shot Mode

```
$ go run ./cmd/client -message "Translate 'hello' to Japanese"
こんにちは (Konnichiwa)
```

### 10.3 Stdin-Pipe Mode

```
$ echo "List 3 Go best practices" | go run ./cmd/client
1. Handle errors explicitly...
2. Use interfaces for abstraction...
3. Prefer composition over inheritance...
```

### 10.4 Streaming Mode

```
$ go run ./cmd/client -stream -message "Count from 1 to 5"
1... 2... 3... 4... 5.
```

### 10.5 Custom Daemon and Model

```
$ go run ./cmd/client \
    -url   http://192.168.32.161:8080 \
    -model gpt-4o-mini \
    -token my-pat-token \
    -system "You are a concise assistant. Keep replies under 20 words." \
    -message "What is Kafka?"
Kafka is a distributed event streaming platform for high-throughput, fault-tolerant data pipelines.
```

### 10.6 Error — Daemon Not Running

```
$ go run ./cmd/client -message "hello"
error: could not connect to http://localhost:8080 — is the daemon running?
exit status 1
```

### 10.7 Error — Bad Token (non-fatal in REPL)

```
# Wrong token in single-shot mode
$ DAEMON_TOKEN=bad-token go run ./cmd/client -message "Hello"
Error: unauthorized — check DAEMON_TOKEN
exit status 1

# REPL — rate limited (non-fatal, loop continues)
$ go run ./cmd/client
Pangreksa AI Gateway Client
  model : gpt-4o
  daemon: http://localhost:8080
Type 'exit' to quit.

> Hello
Error: rate limited — retry after 30s

> Hello again
Assistant: Hello! How can I help you today?

> exit
Goodbye.
```

---

## 11. Risks & Mitigation

| ID       | Risk                                           | Likelihood | Impact | Mitigation                                                      |
|----------|------------------------------------------------|------------|--------|-----------------------------------------------------------------|
| RSK-CLI-001 | Token exposed via `-token` flag in shell history | Medium  | High   | Document env var usage as preferred; README warns against flag  |
| RSK-CLI-002 | SSE stream hangs indefinitely on network stall  | Low     | Medium | HTTP client timeout (`DAEMON_TIMEOUT` default 60s) closes stalled connections |
| RSK-CLI-003 | History grows unboundedly in long REPL sessions  | Low    | Medium | Document: start new session for long conversations; future version may add `-max-history` flag |
| RSK-CLI-004 | Non-TTY detection false positive on some terminals | Low   | Low    | Use `github.com/mattn/go-isatty` (already in `go.mod` as indirect dep) for reliable TTY detection |
| RSK-CLI-005 | Daemon URL has trailing slash causing 404        | Low     | Low    | Config loader normalises URL with `strings.TrimRight(url, "/")` |

---

## 14. Appendix

### 14.1 Glossary

| Term           | Definition                                                                    |
|----------------|-------------------------------------------------------------------------------|
| REPL           | Read-Eval-Print Loop — interactive session: read input, send to daemon, print response, repeat |
| SSE            | Server-Sent Events — HTTP/1.1 streaming where the server sends `data: {...}` lines over a persistent connection |
| Bearer token   | HTTP Authorization credential: `Authorization: Bearer <token>`; used for daemon authentication |
| Single-shot    | CLI invocation mode where one request is made and the process exits immediately |
| Stdin-pipe     | CLI invocation mode where the prompt is read from stdin (non-TTY); enables shell pipeline use |
| TTY            | Teletypewriter — a terminal device; stdin is a TTY in interactive shells and not a TTY in pipes |
| ChatCompletion | OpenAI-compatible API object for conversational LLM requests and responses    |
| Delta          | Partial token content in an SSE streaming chunk: `choices[0].delta.content`  |

### 14.2 Related Go Dependencies (stdlib only)

| Package           | Purpose                                                                   |
|-------------------|---------------------------------------------------------------------------|
| `net/http`        | HTTP client for POST requests and SSE response body streaming             |
| `encoding/json`   | Marshal request payload; unmarshal response JSON and SSE delta chunks     |
| `bufio`           | Line-by-line SSE stream reading; buffered stdin reading for pipe mode     |
| `flag`            | Command-line flag parsing (`-url`, `-model`, `-token`, `-message`, etc.)  |
| `os`              | Environment variable lookup (`os.Getenv`); tty detection; `os.Exit`      |

No third-party Go modules are required. The `go.mod` file for this binary lists no external
`require` entries beyond what already exists in the gateway engine module.

### 14.3 References

| #    | Reference                                                                    |
|------|------------------------------------------------------------------------------|
| R-01 | SRS-AI-GATEWAY-ENGINE-001 v1.0 — Pangreksa AI Gateway Engine               |
| R-02 | OpenAI Chat Completions API — https://platform.openai.com/docs/api-reference/chat |
| R-03 | iSAQB CPSA-A Curriculum — https://www.isaqb.org                             |
| R-04 | ISO/IEC 29148:2018 — Requirements Engineering                                |
| R-05 | Go 1.22 Release Notes — https://go.dev/doc/go1.22                           |
| R-06 | Server-Sent Events Spec — https://html.spec.whatwg.org/multipage/server-sent-events.html |
| R-07 | mattn/go-isatty — https://github.com/mattn/go-isatty (TTY detection, indirect dep) |

---

*End of SRS — Pangreksa AI Gateway Client CLI v1.0*

---

> | Version | Date       | Author                        | Change          |
> |---------|------------|-------------------------------|-----------------|
> | 1.0     | 2026-06-04 | AI Software Architect (iSAQB) | Initial draft   |
