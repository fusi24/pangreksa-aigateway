package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// claudeRequest is the Anthropic Messages API request wire format.
// https://docs.anthropic.com/en/api/messages
type claudeRequest struct {
	Model     string          `json:"model"`
	Messages  []claudeMessage `json:"messages"`
	System    string          `json:"system,omitempty"`
	MaxTokens int             `json:"max_tokens"`
	Stream    bool            `json:"stream"`
	Tools     []claudeTool    `json:"tools,omitempty"`
}

// claudeMessage is a single message in the Anthropic Messages API format.
type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeTool represents a tool definition in Anthropic's format.
type claudeTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

// ClaudeAdapter normalizes Anthropic Messages API requests to InquiryRequest.
// Provider name: "claude".
//
// Thread-safety: safe for concurrent use; ClaudeAdapter holds no mutable state.
type ClaudeAdapter struct{}

// NewClaudeAdapter constructs an Anthropic adapter.
func NewClaudeAdapter() *ClaudeAdapter {
	return &ClaudeAdapter{}
}

// Name returns the canonical provider identifier "claude".
func (a *ClaudeAdapter) Name() string { return "claude" }

// Normalize decodes an Anthropic Messages API HTTP request into an InquiryRequest.
// The system prompt (if any) is prepended as a Message with role "system" so the
// unified pipeline can treat all providers uniformly.
func (a *ClaudeAdapter) Normalize(r *http.Request) (*model.InquiryRequest, error) {
	var body claudeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("claude.Normalize: decode body: %w", err)
	}

	var msgs []model.Message
	// Anthropic sends the system prompt as a top-level "system" field;
	// normalize it to a system role message for uniform handling.
	if body.System != "" {
		msgs = append(msgs, model.Message{Role: "system", Content: body.System})
	}
	for _, m := range body.Messages {
		msgs = append(msgs, model.Message{Role: m.Role, Content: m.Content})
	}

	// convert Anthropic tool definitions to the unified Tool format.
	tools := make([]model.Tool, 0, len(body.Tools))
	for _, t := range body.Tools {
		tools = append(tools, model.Tool{
			Type: "function",
			Function: model.ToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	return &model.InquiryRequest{
		RequestID:  uuid.NewString(),
		UserID:     r.Header.Get("X-Gateway-Token"),
		Provider:   "claude",
		Model:      body.Model,
		Messages:   msgs,
		Tools:      tools,
		Stream:     body.Stream,
		MaxTokens:  body.MaxTokens,
		ReceivedAt: time.Now(),
	}, nil
}

// anthropicResponse is the top-level shape of an Anthropic Messages API response.
// Only the fields needed for OpenAI normalisation are decoded.
type anthropicResponse struct {
	ID         string              `json:"id"`
	Type       string              `json:"type"`
	Role       string              `json:"role"`
	Model      string              `json:"model"`
	Content    []anthropicContent  `json:"content"`
	StopReason string              `json:"stop_reason"`
	Usage      anthropicUsage      `json:"usage"`
}

// anthropicContent is a single content block in an Anthropic response.
type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// anthropicUsage holds Anthropic token counts.
type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// openAICompletionResponse is the OpenAI-compatible response shape the client expects.
type openAICompletionResponse struct {
	ID      string                  `json:"id"`
	Object  string                  `json:"object"`
	Model   string                  `json:"model"`
	Choices []openAICompletionChoice `json:"choices"`
	Usage   openAICompletionUsage    `json:"usage"`
}

// openAICompletionChoice wraps the assistant message in OpenAI response format.
type openAICompletionChoice struct {
	Index        int            `json:"index"`
	Message      openAIMessage  `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

// openAICompletionUsage holds OpenAI-compatible token counts.
type openAICompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// FormatResponse converts an Anthropic Messages API response to the
// OpenAI-compatible chat completion format so clients on any port receive
// a uniform response shape.
func (a *ClaudeAdapter) FormatResponse(providerResp []byte, req *model.InquiryRequest) ([]byte, error) {
	var ar anthropicResponse
	if err := json.Unmarshal(providerResp, &ar); err != nil {
		// Return raw bytes unchanged if the response cannot be decoded.
		return providerResp, nil
	}

	// Concatenate all text content blocks into a single string.
	var sb strings.Builder
	for _, block := range ar.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}

	finishReason := "stop"
	if ar.StopReason != "" {
		finishReason = ar.StopReason
	}

	model := ar.Model
	if model == "" && req != nil {
		model = req.Model
	}

	out := openAICompletionResponse{
		ID:     ar.ID,
		Object: "chat.completion",
		Model:  model,
		Choices: []openAICompletionChoice{
			{
				Index: 0,
				Message: openAIMessage{
					Role:    "assistant",
					Content: sb.String(),
				},
				FinishReason: finishReason,
			},
		},
		Usage: openAICompletionUsage{
			PromptTokens:     ar.Usage.InputTokens,
			CompletionTokens: ar.Usage.OutputTokens,
			TotalTokens:      ar.Usage.InputTokens + ar.Usage.OutputTokens,
		},
	}

	b, err := json.Marshal(out)
	if err != nil {
		return providerResp, fmt.Errorf("claude.FormatResponse: marshal: %w", err)
	}
	return b, nil
}
