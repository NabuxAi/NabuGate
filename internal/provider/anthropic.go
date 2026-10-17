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
	Model         string             `json:"model"`
	MaxTokens     int                `json:"max_tokens"`
	System        string             `json:"system,omitempty"`
	Messages      []anthropicMessage `json:"messages"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"` // "user" | "assistant"
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	StopReason string            `json:"stop_reason"`
	Usage      anthropicUsageRaw `json:"usage"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// anthropicUsageRaw is Anthropic's usage object. The cache directions are
// billed at rates of their own on the real invoice, so they are carried
// separately rather than folded into the input count.
type anthropicUsageRaw struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

// usage normalizes the raw shape into the gateway's Usage.
func (u anthropicUsageRaw) usage() Usage {
	return Usage{
		PromptTokens:       u.InputTokens,
		CompletionTokens:   u.OutputTokens,
		TotalTokens:        u.InputTokens + u.OutputTokens,
		CacheReadInput:     u.CacheReadInputTokens,
		CacheCreationInput: u.CacheCreationInputTokens,
	}
}

// anthropicSampling drops a sampling parameter for reasoning-style targets:
// Claude Opus 5.5 / Fable 5.1 reject temperature and top_p outright, and
// Sonnet 5.5 / Haiku 5.5 reject any non-default value.
func anthropicSampling(req ChatRequest, v *float64) *float64 {
	if req.ParamStyle == ParamStyleReasoning {
		return nil
	}
	return v
}

// mapAnthropicFinish normalizes Anthropic's stop_reason to the OpenAI vocabulary.
func mapAnthropicFinish(r string) string {
	switch r {
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
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
		Model:         req.Model,
		MaxTokens:     maxTokens,
		System:        system.String(),
		Messages:      msgs,
		Temperature:   anthropicSampling(req, req.Temperature),
		TopP:          anthropicSampling(req, req.TopP),
		StopSequences: stopToSlice(req.Stop),
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

	usage := parsed.Usage.usage()
	if usage.TotalTokens == 0 && usage.CacheReadInput == 0 && usage.CacheCreationInput == 0 {
		// Anthropic answered with content but no usage object. Guess from the
		// bytes that moved, flagged as a guess, so the call is metered at
		// something rather than passed off as a free month.
		usage = estimateUsage(req, len(text.String()))
	}

	return ChatResponse{
		Content:      text.String(),
		FinishReason: mapAnthropicFinish(parsed.StopReason),
		Usage:        usage,
	}, nil
}

// ChatStream implements StreamAdapter for the Anthropic Messages API.
func (a *AnthropicAdapter) ChatStream(ctx context.Context, req ChatRequest, onDelta DeltaFunc) (Usage, error) {
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

	payload := map[string]any{
		"model":      req.Model,
		"max_tokens": maxTokens,
		"messages":   msgs,
		"stream":     true,
	}
	if system.Len() > 0 {
		payload["system"] = system.String()
	}
	if t := anthropicSampling(req, req.Temperature); t != nil {
		payload["temperature"] = *t
	}
	if p := anthropicSampling(req, req.TopP); p != nil {
		payload["top_p"] = *p
	}
	if stops := stopToSlice(req.Stop); len(stops) > 0 {
		payload["stop_sequences"] = stops
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Usage{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return Usage{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", a.version)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := doStreamRequest(ctx, httpReq, a.name)
	if err != nil {
		return Usage{}, err
	}
	defer resp.Body.Close()

	var usage Usage
	var outputBytes int
	err = readSSE(resp.Body, func(data []byte) (bool, error) {
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Text         string `json:"text"`
				OutputTokens int    `json:"output_tokens"`
			} `json:"delta"`
			Message struct {
				Usage anthropicUsageRaw `json:"usage"`
			} `json:"message"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(data, &ev) != nil {
			return false, nil
		}
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Text != "" {
				outputBytes += len(ev.Delta.Text)
				if err := onDelta(ev.Delta.Text); err != nil {
					return true, err
				}
			}
		case "message_start":
			usage.PromptTokens = ev.Message.Usage.InputTokens
			usage.CacheReadInput = ev.Message.Usage.CacheReadInputTokens
			usage.CacheCreationInput = ev.Message.Usage.CacheCreationInputTokens
		case "message_delta":
			usage.CompletionTokens = ev.Usage.OutputTokens
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		return false, nil
	})
	if usage.TotalTokens == 0 && usage.CacheReadInput == 0 && usage.CacheCreationInput == 0 {
		// No message_start and no message_delta arrived — the upstream told us
		// nothing about what the call spent. Guess from the bytes on the wire,
		// flagged as a guess, exactly as the OpenAI-wire stream path does.
		usage = estimateUsage(req, outputBytes)
	}
	return usage, err
}
