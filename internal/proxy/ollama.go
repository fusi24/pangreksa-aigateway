package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// ollamaRequest mirrors the OpenAI-compatible chat completion format used by the
// Ollama local inference server. Ollama's /api/chat endpoint accepts an OpenAI-style
// body when using the /v1/chat/completions path.
type ollamaRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Tools       []model.Tool    `json:"tools,omitempty"`
	Stream      bool            `json:"stream"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
}

// OllamaAdapter normalizes Ollama (OpenAI-compatible) API requests to InquiryRequest.
// Provider name: "ollama".
//
// Thread-safety: safe for concurrent use; OllamaAdapter holds no mutable state.
type OllamaAdapter struct{}

// NewOllamaAdapter constructs an Ollama adapter.
func NewOllamaAdapter() *OllamaAdapter {
	return &OllamaAdapter{}
}

// Name returns the canonical provider identifier "ollama".
func (a *OllamaAdapter) Name() string { return "ollama" }

// Normalize decodes an Ollama (OpenAI-format) HTTP request into an InquiryRequest.
func (a *OllamaAdapter) Normalize(r *http.Request) (*model.InquiryRequest, error) {
	var body ollamaRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("ollama.Normalize: decode body: %w", err)
	}

	msgs := make([]model.Message, 0, len(body.Messages))
	for _, m := range body.Messages {
		msgs = append(msgs, model.Message{
			Role:    m.Role,
			Content: m.Content,
			Name:    m.Name,
		})
	}

	return &model.InquiryRequest{
		RequestID:   uuid.NewString(),
		UserID:      r.Header.Get("X-Gateway-Token"),
		Provider:    "ollama",
		Model:       body.Model,
		Messages:    msgs,
		Tools:       body.Tools,
		Stream:      body.Stream,
		Temperature: body.Temperature,
		MaxTokens:   body.MaxTokens,
		ReceivedAt:  time.Now(),
	}, nil
}

// FormatResponse is a pass-through for Ollama — the provider response is returned
// verbatim since clients targeting the Ollama port expect the OpenAI wire format.
func (a *OllamaAdapter) FormatResponse(providerResp []byte, _ *model.InquiryRequest) ([]byte, error) {
	return providerResp, nil
}
