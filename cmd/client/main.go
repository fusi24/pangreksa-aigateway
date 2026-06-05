// Command client is an interactive CLI test client for the Pangreksa AI Gateway Daemon.
// It connects using the OpenAI-compatible chat completion API and supports interactive,
// single-shot, and stdin-pipe modes.
//
// Usage:
//
//	go run ./cmd/client                                          # interactive REPL (OpenAI default)
//	go run ./cmd/client -proxy claude -model claude-sonnet-4-5  # Anthropic via Claude proxy
//	go run ./cmd/client -proxy moonshot -model moonshot-v1-8k   # Moonshot (Kimi)
//	go run ./cmd/client -message "hello"                        # single-shot
//	echo "hello" | go run ./cmd/client                          # stdin pipe
//
// Environment variables (all overridable with flags):
//
//	DAEMON_URL      — gateway daemon base URL (default: http://localhost:8080)
//	DAEMON_MODEL    — model name (default: gpt-4o)
//	DAEMON_TOKEN    — bearer auth token
//	DAEMON_SYSTEM   — optional system prompt
//	DAEMON_PROXY    — proxy name: openai | claude | moonshot | ollama
//	DAEMON_PROVIDER — display label for the provider (cosmetic only)
//	DAEMON_HOST     — daemon hostname when using -proxy (default: localhost)
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// defaultURL is the default gateway daemon base URL (OpenAI proxy port).
const defaultURL = "http://localhost:8080"

// defaultModel is the default model identifier sent in every request.
const defaultModel = "gpt-4o"

// defaultToken is the default bearer authentication token for development.
const defaultToken = "dev-test-pat-token-hash"

// proxyPorts maps proxy name to the daemon's listen port for that provider.
var proxyPorts = map[string]int{
	"openai":   8080,
	"claude":   8081,
	"moonshot": 8082,
	"ollama":   8083,
}

// proxyDefaultModels maps proxy name to the sensible default model for that provider.
var proxyDefaultModels = map[string]string{
	"openai":   "gpt-4o",
	"claude":   "claude-sonnet-4-5",
	"moonshot": "moonshot-v1-8k",
	"ollama":   "llama3",
}

// proxyURL returns the daemon base URL for the named proxy on host.
// Returns "" when the proxy name is not in proxyPorts.
func proxyURL(proxy, host string) string {
	port, ok := proxyPorts[proxy]
	if !ok {
		return ""
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}

// defaultModelForProxy returns the default model identifier for proxy.
// Falls back to the global defaultModel for unknown proxy names.
func defaultModelForProxy(proxy string) string {
	if m, ok := proxyDefaultModels[proxy]; ok {
		return m
	}
	return defaultModel
}

// validProxyNames returns a sorted comma-separated list of known proxy names.
func validProxyNames() string {
	return "openai | claude | moonshot | ollama"
}

// chatMessage represents a single message in the OpenAI-compatible chat history.
// Role is one of "system", "user", or "assistant".
//
// Thread-safety: not safe for concurrent mutation.
type chatMessage struct {
	// Role identifies the participant: "system", "user", or "assistant".
	Role string `json:"role"`
	// Content is the text body of the message.
	Content string `json:"content"`
}

// chatRequest is the request body sent to the OpenAI-compatible
// /v1/chat/completions endpoint.
//
// Thread-safety: not safe for concurrent mutation.
type chatRequest struct {
	// Model is the model identifier to use for this completion.
	Model string `json:"model"`
	// Messages is the full ordered conversation history.
	Messages []chatMessage `json:"messages"`
	// Stream controls whether the response is delivered as SSE tokens.
	Stream bool `json:"stream"`
}

// chatChoice represents a single completion choice in a non-streaming response.
type chatChoice struct {
	// Message holds the assistant's reply for non-streaming responses.
	Message chatMessage `json:"message"`
}

// chatResponse is the top-level non-streaming response from /v1/chat/completions.
type chatResponse struct {
	// Choices contains the list of completion candidates. Index 0 is used.
	Choices []chatChoice `json:"choices"`
}

// streamDelta carries the incremental content in an SSE streaming response chunk.
type streamDelta struct {
	// Content is the incremental text token for this chunk. May be empty.
	Content string `json:"content"`
}

// streamChoice wraps a single delta in a streaming response.
type streamChoice struct {
	// Delta contains the incremental token content for streaming responses.
	Delta streamDelta `json:"delta"`
}

// streamChunk is a single SSE data payload in a streaming /v1/chat/completions
// response. Each line is decoded independently; [DONE] terminates the stream.
type streamChunk struct {
	// Choices contains the streaming delta choices; index 0 is used.
	Choices []streamChoice `json:"choices"`
}

// config holds all runtime configuration for the client, resolved from
// environment variables first and then overridden by command-line flags.
//
// Thread-safety: read-only after parseConfig; safe for concurrent reads.
type config struct {
	// URL is the resolved base URL of the Gateway Daemon (e.g. http://localhost:8081).
	// Set directly via -url, or derived from -proxy + -host.
	URL string
	// Proxy is the named proxy (openai | claude | moonshot | ollama).
	// When set, URL and default Model are derived from the proxy name.
	Proxy string
	// Provider is an optional display label for the upstream provider (cosmetic only).
	Provider string
	// Host is the daemon hostname used when building the URL from -proxy.
	Host string
	// Model is the model name sent in every chat completion request.
	Model string
	// Token is the bearer token used in the Authorization header.
	Token string
	// Message is the optional single-shot message. Empty means interactive/pipe mode.
	Message string
	// System is the optional system prompt prepended to every conversation.
	System string
	// Stream enables SSE streaming when true.
	Stream bool
}

// envOrDefault returns the value of the named environment variable, or
// fallback when the variable is absent or empty.
func envOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// parseConfig reads environment variables and then applies command-line flags,
// so that flags always take precedence over env vars.
// It returns the resolved config and calls flag.Parse internally.
// Exits with code 1 when an unknown -proxy name is supplied.
func parseConfig() *config {
	cfg := &config{}

	// Track whether -model was explicitly set so we can apply proxy default only
	// when the user did not override it.
	explicitModel := envOrDefault("DAEMON_MODEL", "")

	flag.StringVar(&cfg.Proxy, "proxy",
		envOrDefault("DAEMON_PROXY", ""),
		"Proxy name: "+validProxyNames()+" (env: DAEMON_PROXY)")

	flag.StringVar(&cfg.Provider, "provider",
		envOrDefault("DAEMON_PROVIDER", ""),
		"Provider display label, cosmetic only (env: DAEMON_PROVIDER)")

	flag.StringVar(&cfg.Host, "host",
		envOrDefault("DAEMON_HOST", "localhost"),
		"Daemon hostname used with -proxy to build URL (env: DAEMON_HOST)")

	flag.StringVar(&cfg.URL, "url",
		envOrDefault("DAEMON_URL", defaultURL),
		"Gateway Daemon base URL; overridden when -proxy is set (env: DAEMON_URL)")

	flag.StringVar(&cfg.Model, "model",
		envOrDefault("DAEMON_MODEL", defaultModel),
		"Model name (env: DAEMON_MODEL)")

	flag.StringVar(&cfg.Token, "token",
		envOrDefault("DAEMON_TOKEN", defaultToken),
		"Bearer auth token (env: DAEMON_TOKEN)")

	flag.StringVar(&cfg.Message, "message", "",
		"Single-shot message; empty enables interactive or pipe mode")

	flag.StringVar(&cfg.System, "system",
		envOrDefault("DAEMON_SYSTEM", ""),
		"Optional system prompt (env: DAEMON_SYSTEM)")

	flag.BoolVar(&cfg.Stream, "stream", false,
		"Enable SSE streaming")

	flag.Parse()

	// Validate and apply -proxy: derive URL and default model.
	if cfg.Proxy != "" {
		u := proxyURL(cfg.Proxy, cfg.Host)
		if u == "" {
			_, _ = fmt.Fprintf(os.Stderr,
				"error: unknown proxy %q — valid values: %s\n", cfg.Proxy, validProxyNames())
			os.Exit(1)
		}
		// -proxy always wins over -url / DAEMON_URL.
		cfg.URL = u

		// Apply proxy default model only when -model / DAEMON_MODEL was not explicitly set.
		if explicitModel == "" && cfg.Model == defaultModel {
			cfg.Model = defaultModelForProxy(cfg.Proxy)
		}
	}

	// Trim trailing slash so URL + path never produces a double-slash.
	cfg.URL = strings.TrimRight(cfg.URL, "/")

	return cfg
}

// buildMessages constructs the messages slice for the request, prepending an
// optional system message when cfg.System is non-empty, followed by all
// provided history entries and the new userText message.
func buildMessages(cfg *config, history []chatMessage, userText string) []chatMessage {
	var msgs []chatMessage

	if cfg.System != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: cfg.System})
	}

	msgs = append(msgs, history...)
	msgs = append(msgs, chatMessage{Role: "user", Content: userText})

	return msgs
}

// doRequest sends a chat completion request to the daemon and returns the
// assistant's reply text. It dispatches to doStream or doNonStream based on
// cfg.Stream.
//
// ctx — the caller supplies a base context; doRequest adds a 120-second timeout.
// Returns a non-nil error on network failure or non-200 HTTP status.
func doRequest(cfg *config, history []chatMessage, userText string) (string, error) {
	msgs := buildMessages(cfg, history, userText)

	reqBody := chatRequest{
		Model:    cfg.Model,
		Messages: msgs,
		Stream:   cfg.Stream,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("client.doRequest: marshal request: %w", err)
	}

	endpoint := cfg.URL + "/v1/chat/completions"

	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("client.doRequest: build request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.Token)

	httpClient := &http.Client{Timeout: 120 * time.Second}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("client.doRequest: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("client.doRequest: server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if cfg.Stream {
		return readStream(resp.Body)
	}

	return readNonStream(resp.Body)
}

// readNonStream decodes a standard (non-SSE) chat completion response body and
// returns choices[0].message.content.
// Returns an error if the body cannot be decoded or contains no choices.
func readNonStream(body io.Reader) (string, error) {
	var chatResp chatResponse
	if err := json.NewDecoder(body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("client.readNonStream: decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("client.readNonStream: response contained no choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// readStream reads an SSE stream from body, printing each delta.content token
// to stdout without a newline. It prints a trailing newline on [DONE] and returns
// the full assembled response text.
// Returns an error if the stream cannot be read or JSON chunks cannot be parsed.
func readStream(body io.Reader) (string, error) {
	var sb strings.Builder

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()

		// SSE comment or blank line — skip.
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		const prefix = "data: "
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		data := strings.TrimPrefix(line, prefix)

		// Terminal SSE marker — end of stream.
		if strings.TrimSpace(data) == "[DONE]" {
			fmt.Println()
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// Malformed chunk — skip rather than abort.
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		token := chunk.Choices[0].Delta.Content
		if token == "" {
			continue
		}

		fmt.Print(token)
		sb.WriteString(token)
	}

	if err := scanner.Err(); err != nil {
		return sb.String(), fmt.Errorf("client.readStream: scan body: %w", err)
	}

	return sb.String(), nil
}

// isTTY reports whether the given file is connected to a terminal (interactive).
// It uses os.File.Stat to inspect the file mode bits without importing syscall.
func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	// ModeCharDevice is set for /dev/tty and Windows console handles.
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// runSingleShot sends cfg.Message once, prints the plain response, and returns
// an exit code (0 success, 1 error).
func runSingleShot(cfg *config) int {
	reply, err := doRequest(cfg, nil, cfg.Message)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Println(reply)
	return 0
}

// runPipe reads all lines from stdin, joins them with newlines, sends as a
// single message, prints the plain response, and returns an exit code.
func runPipe(cfg *config) int {
	scanner := bufio.NewScanner(os.Stdin)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
		return 1
	}

	message := strings.Join(lines, "\n")
	if strings.TrimSpace(message) == "" {
		_, _ = fmt.Fprintln(os.Stderr, "error: no input provided via stdin")
		return 1
	}

	reply, err := doRequest(cfg, nil, message)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Println(reply)
	return 0
}

// runInteractive starts the interactive REPL loop, maintaining conversation
// history across turns. The loop exits when the user types "exit" or "quit",
// or when stdin is closed (EOF). Errors from individual requests are printed
// but do not terminate the loop.
func runInteractive(cfg *config) int {
	fmt.Println("Pangreksa AI Gateway Client")
	if cfg.Proxy != "" {
		fmt.Printf("  proxy    : %s  →  %s\n", cfg.Proxy, cfg.URL)
	} else {
		fmt.Printf("  url      : %s\n", cfg.URL)
	}
	if cfg.Provider != "" {
		fmt.Printf("  provider : %s\n", cfg.Provider)
	}
	fmt.Printf("  model    : %s\n", cfg.Model)
	fmt.Printf("  token    : %s...\n", cfg.Token[:min(len(cfg.Token), 12)])
	fmt.Println("Type 'exit' or 'quit' to stop.")
	fmt.Println()

	var history []chatMessage

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")

		if !scanner.Scan() {
			// EOF or error — exit cleanly.
			fmt.Println()
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if line == "exit" || line == "quit" {
			fmt.Println("Goodbye.")
			break
		}

		fmt.Printf("\nYou: %s\n", line)

		// For streaming mode, the tokens are printed inside readStream;
		// we still need to print the "Assistant: " prefix before calling doRequest.
		if cfg.Stream {
			fmt.Print("Assistant: ")
		}

		reply, err := doRequest(cfg, history, line)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n\n", err)
			continue
		}

		if !cfg.Stream {
			fmt.Printf("Assistant: %s\n", reply)
		}
		fmt.Println()

		// Append the exchange to history so subsequent turns have full context.
		history = append(history,
			chatMessage{Role: "user", Content: line},
			chatMessage{Role: "assistant", Content: reply},
		)
	}

	return 0
}

func main() {
	cfg := parseConfig()

	var exitCode int

	switch {
	case cfg.Message != "":
		// Single-shot mode: -message flag provided.
		exitCode = runSingleShot(cfg)

	case !isTTY(os.Stdin):
		// Pipe mode: stdin is not a terminal.
		exitCode = runPipe(cfg)

	default:
		// Interactive REPL mode.
		exitCode = runInteractive(cfg)
	}

	os.Exit(exitCode)
}
