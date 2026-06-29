package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// OpenAIAdapter speaks the OpenAI Chat Completions API. Because OpenAI,
// Groq and OpenRouter are all wire-compatible, this single adapter serves all
// of them — only base_url, api_key and (optionally) extra headers differ.
type OpenAIAdapter struct {
	name         string
	baseURL      string
	apiKey       string
	extraHeaders map[string]string
}

// NewOpenAIAdapter builds an OpenAI-compatible adapter.
func NewOpenAIAdapter(name, baseURL, apiKey string, extraHeaders map[string]string) *OpenAIAdapter {
	return &OpenAIAdapter{
		name:         name,
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       apiKey,
		extraHeaders: extraHeaders,
	}
}

func (a *OpenAIAdapter) Name() string { return a.name }

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content   string          `json:"content"`
			ToolCalls json.RawMessage `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// buildBody assembles the upstream request. When the caller supplies the raw
// OpenAI-style body (req.Raw), it is forwarded verbatim — only "model" and the
// streaming flags are overridden — so every OpenAI parameter (tools,
// tool_choice, response_format, seed, penalties, …) passes through untouched.
// If req.Raw is empty (internal callers/tests), the body is built from the
// typed fields instead.
func (a *OpenAIAdapter) buildBody(req ChatRequest, stream bool) ([]byte, error) {
	out := make(map[string]json.RawMessage, len(req.Raw)+3)
	for k, v := range req.Raw {
		out[k] = v
	}
	if len(out) == 0 {
		msgs, err := json.Marshal(req.Messages)
		if err != nil {
			return nil, err
		}
		out["messages"] = msgs
		if req.Temperature != nil {
			out["temperature"], _ = json.Marshal(*req.Temperature)
		}
		if req.TopP != nil {
			out["top_p"], _ = json.Marshal(*req.TopP)
		}
		if req.MaxTokens != nil {
			out["max_tokens"], _ = json.Marshal(*req.MaxTokens)
		}
		if len(req.Stop) > 0 {
			out["stop"] = req.Stop
		}
	}
	out["model"], _ = json.Marshal(req.Model)
	if stream {
		out["stream"] = json.RawMessage("true")
		out["stream_options"] = json.RawMessage(`{"include_usage":true}`)
	} else {
		delete(out, "stream")
		delete(out, "stream_options")
	}
	return json.Marshal(out)
}

func (a *OpenAIAdapter) headers() map[string]string {
	h := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + a.apiKey,
	}
	for k, v := range a.extraHeaders {
		h[k] = v
	}
	return h
}

func (a *OpenAIAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	body, err := a.buildBody(req, false)
	if err != nil {
		return ChatResponse{}, err
	}

	status, raw, err := postJSON(ctx, a.baseURL+"/chat/completions", a.headers(), body, a.name)
	if err != nil {
		return ChatResponse{}, err
	}

	var parsed openAIChatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ChatResponse{}, fmt.Errorf("%s: invalid response (status %d): %s", a.name, status, truncate(raw))
	}
	if status >= 400 {
		msg := http.StatusText(status)
		if parsed.Error != nil && parsed.Error.Message != "" {
			msg = parsed.Error.Message
		}
		return ChatResponse{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, status, msg)
	}
	if len(parsed.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("%s: empty completion", a.name)
	}

	choice := parsed.Choices[0]
	return ChatResponse{
		Content:      choice.Message.Content,
		ToolCalls:    choice.Message.ToolCalls,
		FinishReason: choice.FinishReason,
		Usage: Usage{
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
		},
	}, nil
}

// ChatStream implements StreamAdapter for OpenAI-compatible providers. The full
// request body is forwarded (so tools/response_format/etc. still apply); only
// content deltas are surfaced through onDelta.
func (a *OpenAIAdapter) ChatStream(ctx context.Context, req ChatRequest, onDelta DeltaFunc) (Usage, error) {
	body, err := a.buildBody(req, true)
	if err != nil {
		return Usage{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Usage{}, err
	}
	for k, v := range a.headers() {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := doStreamRequest(ctx, httpReq, a.name)
	if err != nil {
		return Usage{}, err
	}
	defer resp.Body.Close()

	var usage Usage
	err = readSSE(resp.Body, func(data []byte) (bool, error) {
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(data, &chunk) != nil {
			return false, nil // skip unparsable keep-alive lines
		}
		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				if err := onDelta(c.Delta.Content); err != nil {
					return true, err
				}
			}
		}
		if chunk.Usage != nil {
			usage = Usage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
		}
		return false, nil
	})
	return usage, err
}

func truncate(b []byte) string {
	const max = 300
	s := strings.TrimSpace(string(b))
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
