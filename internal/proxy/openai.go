package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// openAIRequest is the wire format for OpenAI chat completion requests.
// Fields not needed by InquiryRequest are intentionally omitted.
type openAIRequest struct {
	Model       string           `json:"model"`
	Messages    []openAIMessage  `json:"messages"`
	Tools       []model.Tool     `json:"tools,omitempty"`
	Stream      bool             `json:"stream"`
	Temperature float64          `json:"temperature"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	User        string           `json:"user,omitempty"`
}

// openAIMessage is a single message in the OpenAI chat format.
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// OpenAIAdapter normalizes OpenAI chat completion requests to InquiryRequest.
// Provider name: "openai".
//
// Thread-safety: safe for concurrent use; OpenAIAdapter holds no mutable state.
type OpenAIAdapter struct{}

// NewOpenAIAdapter constructs an OpenAI adapter.
func NewOpenAIAdapter() *OpenAIAdapter {
	return &OpenAIAdapter{}
}

// Name returns the canonical provider identifier "openai".
func (a *OpenAIAdapter) Name() string { return "openai" }

// Normalize decodes an OpenAI chat completion HTTP request into an InquiryRequest.
// The UserID and OrgID fields are extracted from the Authorization bearer token
// stored in X-Gateway-Token by the proxy Server; they are left empty here and
// populated by the entitlement resolver in the pipeline.
func (a *OpenAIAdapter) Normalize(r *http.Request) (*model.InquiryRequest, error) {
	var body openAIRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("openai.Normalize: decode body: %w", err)
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
		Provider:    "openai",
		Model:       body.Model,
		Messages:    msgs,
		Tools:       body.Tools,
		Stream:      body.Stream,
		Temperature: body.Temperature,
		MaxTokens:   body.MaxTokens,
		ReceivedAt:  time.Now(),
	}, nil
}

// FormatResponse is a pass-through for OpenAI — the provider response is returned
// verbatim since clients targeting the OpenAI port expect the OpenAI wire format.
func (a *OpenAIAdapter) FormatResponse(providerResp []byte, _ *model.InquiryRequest) ([]byte, error) {
	return providerResp, nil
}
