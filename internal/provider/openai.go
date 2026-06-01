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

type openAIChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
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

func (a *OpenAIAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	body, err := json.Marshal(openAIChatRequest{
		Model:       req.Model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	})
	if err != nil {
		return ChatResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	for k, v := range a.extraHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return ChatResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var parsed openAIChatResponse
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
	if len(parsed.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("%s: empty completion", a.name)
	}

	return ChatResponse{
		Content: parsed.Choices[0].Message.Content,
		Usage: Usage{
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
		},
	}, nil
}

// ChatStream implements StreamAdapter for OpenAI-compatible providers.
func (a *OpenAIAdapter) ChatStream(ctx context.Context, req ChatRequest, onDelta DeltaFunc) (Usage, error) {
	payload := map[string]any{
		"model":          req.Model,
		"messages":       req.Messages,
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		payload["max_tokens"] = *req.MaxTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Usage{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Usage{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	for k, v := range a.extraHeaders {
		httpReq.Header.Set(k, v)
	}

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
