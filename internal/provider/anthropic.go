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
// OpenAI shape in that the system prompt is a top-level field, max_tokens is
// required, auth uses the x-api-key header, and tools, tool calls, tool
// results and images are content blocks; anthropic_wire.go translates.
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

// SupportsTools implements ToolCapableAdapter: OpenAI tools, tool_choice, tool
// calls and tool results are translated to and from Anthropic's tool_use and
// tool_result blocks (anthropic_wire.go).
func (a *AnthropicAdapter) SupportsTools() bool { return true }

type anthropicResponse struct {
	Content    []anthropicBlock  `json:"content"`
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
	body, err := buildAnthropicBody(req, false)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("%s: %w", a.name, err)
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
		if c.Type == "" || c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	toolCalls := openAIToolCalls(parsed.Content)

	usage := parsed.Usage.usage()
	if usage.TotalTokens == 0 && usage.CacheReadInput == 0 && usage.CacheCreationInput == 0 {
		// Anthropic answered with content but no usage object. Guess from the
		// bytes that moved, flagged as a guess, so the call is metered at
		// something rather than passed off as a free month.
		usage = estimateUsage(req, len(text.String())+len(toolCalls))
	}

	return ChatResponse{
		Content:      text.String(),
		ToolCalls:    toolCalls,
		FinishReason: mapAnthropicFinish(parsed.StopReason),
		Usage:        usage,
	}, nil
}

// ChatStream implements StreamAdapter for the Anthropic Messages API.
func (a *AnthropicAdapter) ChatStream(ctx context.Context, req ChatRequest, onDelta DeltaFunc) (Usage, error) {
	body, err := buildAnthropicBody(req, true)
	if err != nil {
		return Usage{}, fmt.Errorf("%s: %w", a.name, err)
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
	onTools := ToolDeltas(ctx)
	// Anthropic numbers every content block; OpenAI numbers tool calls only.
	// toolIndex maps one to the other for the deltas the caller reassembles.
	toolIndex := map[int]int{}
	err = readSSE(resp.Body, func(data []byte) (bool, error) {
		var ev struct {
			Type         string         `json:"type"`
			Index        int            `json:"index"`
			ContentBlock anthropicBlock `json:"content_block"`
			Delta        struct {
				Type         string `json:"type"`
				Text         string `json:"text"`
				PartialJSON  string `json:"partial_json"`
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
		case "content_block_start":
			if ev.ContentBlock.Type == "tool_use" && onTools != nil {
				i := len(toolIndex)
				toolIndex[ev.Index] = i
				start, _ := json.Marshal([]map[string]any{{
					"index": i, "id": ev.ContentBlock.ID, "type": "function",
					"function": map[string]any{"name": ev.ContentBlock.Name, "arguments": ""},
				}})
				outputBytes += len(ev.ContentBlock.Name)
				if err := onTools(start); err != nil {
					return true, err
				}
			}
		case "content_block_delta":
			if ev.Delta.Type == "input_json_delta" {
				i, ok := toolIndex[ev.Index]
				if !ok || onTools == nil || ev.Delta.PartialJSON == "" {
					return false, nil
				}
				part, _ := json.Marshal([]map[string]any{{"index": i, "function": map[string]any{"arguments": ev.Delta.PartialJSON}}})
				outputBytes += len(ev.Delta.PartialJSON)
				if err := onTools(part); err != nil {
					return true, err
				}
				return false, nil
			}
			if ev.Delta.Type != "" && ev.Delta.Type != "text_delta" {
				return false, nil // thinking and signature deltas are not the answer
			}
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
