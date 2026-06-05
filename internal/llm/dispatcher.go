package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/pkg/crypto"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// providerConfig holds the decrypted runtime configuration for a single LLM provider.
type providerConfig struct {
	// baseURL is the upstream LLM provider API base URL.
	baseURL string
	// apiKey is the plaintext provider API key (decrypted from ProxyConfig.APIKeyEnc).
	apiKey string
}

// Dispatcher routes InquiryRequests to the appropriate LLM provider.
// It decrypts provider API keys at load time using the supplied crypto key,
// integrates the circuit breaker, and sends HTTP POST requests to the upstream.
//
// Thread-safety: safe for concurrent use; config reloads use a write lock.
type Dispatcher struct {
	// mu guards the providers map during hot-reload.
	mu sync.RWMutex
	// providers maps provider name → providerConfig.
	providers map[string]providerConfig
	// cryptoKey is the AES-256 key used to decrypt APIKeyEnc fields.
	cryptoKey []byte
	// cb is the circuit breaker checked before every dispatch.
	cb *CircuitBreaker
	// httpClient is reused for connection pooling.
	httpClient *http.Client
	// log is the structured logger.
	log zerolog.Logger
}

// NewDispatcher constructs a Dispatcher and decrypts provider API keys from cfg.
//
// cryptoKey must be the 32-byte AES-256 key from pkg/crypto.KeyFromHex.
// cb is the shared circuit breaker for all providers.
func NewDispatcher(cfg model.GatewayConfig, cryptoKey []byte, cb *CircuitBreaker, log zerolog.Logger) *Dispatcher {
	d := &Dispatcher{
		cryptoKey:  cryptoKey,
		cb:         cb,
		httpClient: &http.Client{Timeout: 120 * time.Second},
		log:        log.With().Str("component", "dispatcher").Logger(),
	}
	d.providers = d.buildProviders(cfg)
	return d
}

// Update hot-reloads provider configurations from a new GatewayConfig.
// In-flight requests complete against the old provider map.
func (d *Dispatcher) Update(cfg model.GatewayConfig) {
	newProviders := d.buildProviders(cfg)

	d.mu.Lock()
	d.providers = newProviders
	d.mu.Unlock()

	d.log.Info().Int("count", len(newProviders)).Msg("dispatcher provider configs updated")
}

// Dispatch sends the InquiryRequest to the target provider and returns the raw
// response bytes. It checks the circuit breaker before dispatching and records
// the outcome for future circuit-breaker state transitions.
//
// Returns an error wrapping ErrCircuitOpen when the provider's breaker is open.
func (d *Dispatcher) Dispatch(ctx context.Context, req *model.InquiryRequest) ([]byte, error) {
	if !d.cb.Allow(req.Provider) {
		return nil, fmt.Errorf("dispatcher.Dispatch: provider=%q: circuit breaker open", req.Provider)
	}

	d.mu.RLock()
	pc, ok := d.providers[req.Provider]
	d.mu.RUnlock()

	if !ok {
		d.cb.RecordFailure(req.Provider)
		return nil, fmt.Errorf("dispatcher.Dispatch: unknown provider %q", req.Provider)
	}

	respBytes, err := d.send(ctx, pc, req)
	if err != nil {
		d.cb.RecordFailure(req.Provider)
		return nil, fmt.Errorf("dispatcher.Dispatch: send: %w", err)
	}

	d.cb.RecordSuccess(req.Provider)
	return respBytes, nil
}

// providerEndpoint returns the correct API path suffix for a given provider.
// OpenAI, Moonshot, and Ollama use the OpenAI-compatible /v1/chat/completions path.
// Anthropic uses /v1/messages with a different request schema.
func providerEndpoint(provider string) string {
	if provider == "claude" {
		return "/v1/messages"
	}
	return "/v1/chat/completions"
}

// openAIBody is the provider-native request shape sent to OpenAI-compatible endpoints.
// Using a dedicated struct (rather than marshalling InquiryRequest directly) ensures
// only the fields the provider understands are included in the HTTP body.
type openAIBody struct {
	Model       string            `json:"model"`
	Messages    []model.Message   `json:"messages"`
	Tools       []model.Tool      `json:"tools,omitempty"`
	Stream      bool              `json:"stream"`
	Temperature float64           `json:"temperature,omitempty"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
}

// claudeBody is the provider-native request shape for the Anthropic Messages API.
type claudeBody struct {
	Model     string          `json:"model"`
	Messages  []model.Message `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
	Stream    bool            `json:"stream"`
}

// buildBody serialises an InquiryRequest into the provider's expected JSON format.
func buildBody(req *model.InquiryRequest) ([]byte, error) {
	var payload interface{}
	if req.Provider == "claude" {
		maxTok := req.MaxTokens
		if maxTok == 0 {
			maxTok = 1024
		}
		payload = claudeBody{
			Model:     req.Model,
			Messages:  req.Messages,
			MaxTokens: maxTok,
			Stream:    req.Stream,
		}
	} else {
		payload = openAIBody{
			Model:       req.Model,
			Messages:    req.Messages,
			Tools:       req.Tools,
			Stream:      req.Stream,
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
		}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("buildBody: %w", err)
	}
	return b, nil
}

// send performs the HTTP POST to the upstream provider API.
func (d *Dispatcher) send(ctx context.Context, pc providerConfig, req *model.InquiryRequest) ([]byte, error) {
	body, err := buildBody(req)
	if err != nil {
		return nil, fmt.Errorf("dispatcher.send: marshal request: %w", err)
	}

	url := pc.baseURL + providerEndpoint(req.Provider)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("dispatcher.send: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.Provider == "claude" {
		// Anthropic requires x-api-key and anthropic-version headers.
		httpReq.Header.Set("x-api-key", pc.apiKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+pc.apiKey)
	}

	resp, err := d.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("dispatcher.send: http: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("dispatcher.send: read body: %w", err)
	}

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("dispatcher.send: upstream error status %d: %s", resp.StatusCode, string(respBytes))
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("dispatcher.send: client error status %d: %s", resp.StatusCode, string(respBytes))
	}

	return respBytes, nil
}

// buildProviders builds the providers map from a GatewayConfig, decrypting API keys.
func (d *Dispatcher) buildProviders(cfg model.GatewayConfig) map[string]providerConfig {
	providers := make(map[string]providerConfig, len(cfg.Proxies))
	for _, proxy := range cfg.Proxies {
		apiKey, err := d.decryptAPIKey(proxy.APIKeyEnc)
		if err != nil {
			d.log.Error().Err(err).Str("provider", proxy.Provider).Msg("failed to decrypt API key; provider disabled")
			continue
		}
		providers[proxy.Provider] = providerConfig{
			baseURL: proxy.BaseURL,
			apiKey:  apiKey,
		}
	}
	return providers
}

// decryptAPIKey decodes the base64-encoded ciphertext and decrypts it with AES-256-GCM.
// If EncryptKeyHex was not provided (cryptoKey is nil/empty) the raw value is used.
func (d *Dispatcher) decryptAPIKey(apiKeyEnc string) (string, error) {
	if len(d.cryptoKey) == 0 || apiKeyEnc == "" {
		// no crypto key configured — treat the value as plaintext.
		return apiKeyEnc, nil
	}

	ciphertext, err := base64.StdEncoding.DecodeString(apiKeyEnc)
	if err != nil {
		// try raw bytes before giving up.
		ciphertext = []byte(apiKeyEnc)
	}

	plaintext, err := crypto.Decrypt(d.cryptoKey, ciphertext)
	if err != nil {
		return "", fmt.Errorf("dispatcher.decryptAPIKey: %w", err)
	}

	return string(plaintext), nil
}
