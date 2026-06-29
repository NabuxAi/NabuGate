package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// GeminiAdapter speaks the Google Gemini generateContent API. Gemini uses
// "user"/"model" roles, a separate systemInstruction field, and passes the API
// key as a query parameter.
type GeminiAdapter struct {
	name    string
	baseURL string
	apiKey  string
}

// NewGeminiAdapter builds a Gemini adapter.
func NewGeminiAdapter(name, baseURL, apiKey string) *GeminiAdapter {
	return &GeminiAdapter{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}
}

func (a *GeminiAdapter) Name() string { return a.name }

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Contents          []geminiContent `json:"contents"`
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenCfg   `json:"generationConfig,omitempty"`
}

type geminiGenCfg struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

// geminiGenConfig builds a generationConfig from the request, or nil if no
// generation parameters were set.
func geminiGenConfig(req ChatRequest) *geminiGenCfg {
	stops := stopToSlice(req.Stop)
	if req.Temperature == nil && req.TopP == nil && req.MaxTokens == nil && len(stops) == 0 {
		return nil
	}
	return &geminiGenCfg{
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		MaxOutputTokens: req.MaxTokens,
		StopSequences:   stops,
	}
}

// mapGeminiFinish normalizes Gemini's finishReason to the OpenAI vocabulary.
func mapGeminiFinish(r string) string {
	switch r {
	case "MAX_TOKENS":
		return "length"
	default:
		return "stop"
	}
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (a *GeminiAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	var system *geminiContent
	contents := make([]geminiContent, 0, len(req.Messages))
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			if system == nil {
				system = &geminiContent{Parts: []geminiPart{{Text: m.Content}}}
			} else {
				system.Parts = append(system.Parts, geminiPart{Text: m.Content})
			}
		case "assistant":
			contents = append(contents, geminiContent{Role: "model", Parts: []geminiPart{{Text: m.Content}}})
		default:
			contents = append(contents, geminiContent{Role: "user", Parts: []geminiPart{{Text: m.Content}}})
		}
	}

	genCfg := geminiGenConfig(req)

	body, err := json.Marshal(geminiRequest{
		Contents:          contents,
		SystemInstruction: system,
		GenerationConfig:  genCfg,
	})
	if err != nil {
		return ChatResponse{}, err
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", a.baseURL, url.PathEscape(req.Model), url.QueryEscape(a.apiKey))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return ChatResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var parsed geminiResponse
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
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return ChatResponse{}, fmt.Errorf("%s: empty completion", a.name)
	}

	var text strings.Builder
	for _, p := range parsed.Candidates[0].Content.Parts {
		text.WriteString(p.Text)
	}

	return ChatResponse{
		Content:      text.String(),
		FinishReason: mapGeminiFinish(parsed.Candidates[0].FinishReason),
		Usage: Usage{
			PromptTokens:     parsed.UsageMetadata.PromptTokenCount,
			CompletionTokens: parsed.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      parsed.UsageMetadata.TotalTokenCount,
		},
	}, nil
}

// ChatStream implements StreamAdapter using Gemini's streamGenerateContent.
func (a *GeminiAdapter) ChatStream(ctx context.Context, req ChatRequest, onDelta DeltaFunc) (Usage, error) {
	var system *geminiContent
	contents := make([]geminiContent, 0, len(req.Messages))
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			if system == nil {
				system = &geminiContent{Parts: []geminiPart{{Text: m.Content}}}
			} else {
				system.Parts = append(system.Parts, geminiPart{Text: m.Content})
			}
		case "assistant":
			contents = append(contents, geminiContent{Role: "model", Parts: []geminiPart{{Text: m.Content}}})
		default:
			contents = append(contents, geminiContent{Role: "user", Parts: []geminiPart{{Text: m.Content}}})
		}
	}

	genCfg := geminiGenConfig(req)
	body, err := json.Marshal(geminiRequest{Contents: contents, SystemInstruction: system, GenerationConfig: genCfg})
	if err != nil {
		return Usage{}, err
	}

	endpoint := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", a.baseURL, url.PathEscape(req.Model), url.QueryEscape(a.apiKey))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Usage{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := doStreamRequest(ctx, httpReq, a.name)
	if err != nil {
		return Usage{}, err
	}
	defer resp.Body.Close()

	var usage Usage
	err = readSSE(resp.Body, func(data []byte) (bool, error) {
		var chunk struct {
			Candidates []struct {
				Content struct {
					Parts []geminiPart `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
			UsageMetadata struct {
				PromptTokenCount     int `json:"promptTokenCount"`
				CandidatesTokenCount int `json:"candidatesTokenCount"`
				TotalTokenCount      int `json:"totalTokenCount"`
			} `json:"usageMetadata"`
		}
		if json.Unmarshal(data, &chunk) != nil {
			return false, nil
		}
		for _, c := range chunk.Candidates {
			for _, p := range c.Content.Parts {
				if p.Text != "" {
					if err := onDelta(p.Text); err != nil {
						return true, err
					}
				}
			}
		}
		if chunk.UsageMetadata.TotalTokenCount > 0 {
			usage = Usage{
				PromptTokens:     chunk.UsageMetadata.PromptTokenCount,
				CompletionTokens: chunk.UsageMetadata.CandidatesTokenCount,
				TotalTokens:      chunk.UsageMetadata.TotalTokenCount,
			}
		}
		return false, nil
	})
	return usage, err
}
