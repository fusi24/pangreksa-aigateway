package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// moonshotRequest mirrors the OpenAI chat completion format used by the Moonshot
// (Kimi) API. The struct is kept separate from openAIRequest to allow for
// Moonshot-specific fields to be added without affecting the OpenAI adapter.
type moonshotRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Tools       []model.Tool    `json:"tools,omitempty"`
	Stream      bool            `json:"stream"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
}

// MoonshotAdapter normalizes Moonshot (Kimi) API requests to InquiryRequest.
// Moonshot uses the same wire format as OpenAI. Provider name: "moonshot".
//
// Thread-safety: safe for concurrent use; MoonshotAdapter holds no mutable state.
type MoonshotAdapter struct{}

// NewMoonshotAdapter constructs a Moonshot adapter.
func NewMoonshotAdapter() *MoonshotAdapter {
	return &MoonshotAdapter{}
}

// Name returns the canonical provider identifier "moonshot".
func (a *MoonshotAdapter) Name() string { return "moonshot" }

// Normalize decodes a Moonshot (OpenAI-format) HTTP request into an InquiryRequest.
func (a *MoonshotAdapter) Normalize(r *http.Request) (*model.InquiryRequest, error) {
	var body moonshotRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("moonshot.Normalize: decode body: %w", err)
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
		Provider:    "moonshot",
		Model:       body.Model,
		Messages:    msgs,
		Tools:       body.Tools,
		Stream:      body.Stream,
		Temperature: body.Temperature,
		MaxTokens:   body.MaxTokens,
		ReceivedAt:  time.Now(),
	}, nil
}

// FormatResponse is a pass-through for Moonshot — the provider response is returned
// verbatim since clients targeting the Moonshot port expect the OpenAI wire format.
func (a *MoonshotAdapter) FormatResponse(providerResp []byte, _ *model.InquiryRequest) ([]byte, error) {
	return providerResp, nil
}
