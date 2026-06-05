# Pangreksa AI Gateway Client

An interactive CLI test client for the Pangreksa AI Gateway Daemon. It communicates
with the daemon using the OpenAI-compatible `/v1/chat/completions` API — the same
format used by Ollama and the OpenAI SDK.

---

## Purpose

The client lets you exercise the Gateway Daemon end-to-end without needing a full
application. Use it to:

- Verify the daemon is running and accepting requests.
- Test bearer token authentication.
- Confirm model routing and policy enforcement (guardrails, budgets, rate limits).
- Inspect streaming vs. non-streaming response behaviour.

---

## Three Modes

### 1. Interactive REPL (default)

Launched when stdin is a terminal and `-message` is not provided. Maintains full
conversation history across turns so context carries over.

```
go run ./cmd/client

Pangreksa AI Gateway Client — interactive mode
Type 'exit' or 'quit' to stop.

> hello
You: hello
Assistant: Hello! How can I help you today?

> what did I just say?
You: what did I just say?
Assistant: You said "hello".

> exit
Goodbye.
```

### 2. Single-shot (`-message "text"`)

Sends exactly one message, prints the plain response, and exits 0.
No labels — suitable for scripting.

```
go run ./cmd/client -message "Summarise the Pangreksa gateway in one sentence."
```

### 3. Stdin pipe

When stdin is not a terminal (piped input), all lines are collected and sent as
a single message. The plain response is printed and the process exits.

```
echo "List three use-cases for an AI gateway." | go run ./cmd/client
```

```
cat questions.txt | go run ./cmd/client
```

---

## Configuration

All options can be set via environment variable or command-line flag.
**Flags take precedence over environment variables.**

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `-url` | `DAEMON_URL` | `http://localhost:8080` | Daemon base URL |
| `-model` | `DAEMON_MODEL` | `gpt-4o` | Model name sent in every request |
| `-token` | `DAEMON_TOKEN` | `dev-test-pat-token-hash` | Bearer auth token |
| `-message` | — | `""` | Single-shot message (empty = interactive / pipe) |
| `-system` | `DAEMON_SYSTEM` | `""` | Optional system prompt prepended to every conversation |
| `-stream` | — | `false` | Enable SSE streaming |

---

## How to Run

### Using `go run` (recommended for development)

```powershell
# Interactive mode
go run ./cmd/client

# Single-shot
go run ./cmd/client -message "ping"

# With a system prompt
go run ./cmd/client -system "You are a concise assistant." -message "What is 2+2?"

# Streaming
go run ./cmd/client -stream -message "Tell me a short story."

# Using a different daemon URL
go run ./cmd/client -url http://localhost:11434 -model llama3
```

### Using the PowerShell launcher script

```powershell
.\scripts\run-client.ps1
```

This script sets `DAEMON_URL`, `DAEMON_MODEL`, and `DAEMON_TOKEN` to dev defaults
and launches the client in interactive mode.

### Build a standalone binary

```powershell
go build -o bin/client ./cmd/client

.\bin\client -message "hello"
```

Or via Makefile:

```powershell
make build-client
```

---

## Example Interactive Session

```
$ go run ./cmd/client -system "You are a helpful assistant."

Pangreksa AI Gateway Client — interactive mode
Type 'exit' or 'quit' to stop.

> What is the Pangreksa AI Gateway?
You: What is the Pangreksa AI Gateway?
Assistant: The Pangreksa AI Gateway is a production-grade daemon engine that acts as
a reverse proxy for multiple LLM providers, enforcing policies like rate limiting,
budget controls, and guardrails while logging all transactions.

> What providers does it support?
You: What providers does it support?
Assistant: It supports OpenAI, Anthropic Claude, Moonshot, and Ollama out of the box.

> exit
Goodbye.
```

---

## Prerequisites

- Go 1.21 or later
- The Gateway Daemon must be running (`.\scripts\run-daemon.ps1`)
- A valid bearer token configured in the daemon's entitlement store
