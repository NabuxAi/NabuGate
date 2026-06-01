package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AnthropicAdapter speaks the Anthropic Messages API. It differs from the
// OpenAI shape in three ways: the system prompt is a top-level field, max_tokens
// is required, and auth uses the x-api-key header.
type AnthropicAdapter struct {
	name    string
	baseURL string
	apiKey  string
	version string
}

// NewAnthropicAdapter builds an Anthropic adapter.
func NewAnthropicAdapter(name, baseURL, apiKey string) *AnthropicAdapter {
	return &AnthropicAdapter{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		version: "2023-06-01",
	}
}

func (a *AnthropicAdapter) Name() string { return a.name }

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Temperature *float64           `json:"temperature,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"` // "user" | "assistant"
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (a *AnthropicAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	// Anthropic carries the system prompt outside the messages array.
	var system strings.Builder
	msgs := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "system" {
			if system.Len() > 0 {
				system.WriteString("\n\n")
			}
			system.WriteString(m.Content)
			continue
		}
		msgs = append(msgs, anthropicMessage{Role: m.Role, Content: m.Content})
	}

	maxTokens := 1024
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		maxTokens = *req.MaxTokens
	}

	body, err := json.Marshal(anthropicRequest{
		Model:       req.Model,
		MaxTokens:   maxTokens,
		System:      system.String(),
		Messages:    msgs,
		Temperature: req.Temperature,
	})
	if err != nil {
		return ChatResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", a.version)

	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return ChatResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var parsed anthropicResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ChatResponse{}, fmt.Errorf("%s: invalid response (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}
	if resp.StatusCode >= 400 {
		msg := http.StatusText(resp.StatusCode)
		if parsed.Error != nil && parsed.Error.Message != "" {
			msg = parsed.Error.Message
		}
		return ChatResponse{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, resp.StatusCode, msg)
	}
	if len(parsed.Content) == 0 {
		return ChatResponse{}, fmt.Errorf("%s: empty completion", a.name)
	}

	var text strings.Builder
	for _, c := range parsed.Content {
		text.WriteString(c.Text)
	}

	return ChatResponse{
		Content: text.String(),
		Usage: Usage{
			PromptTokens:     parsed.Usage.InputTokens,
			CompletionTokens: parsed.Usage.OutputTokens,
			TotalTokens:      parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
		},
	}, nil
}
