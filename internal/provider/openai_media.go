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

// --- Image generation (OpenAI Images API) ---

type openAIImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type openAIImageResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Image implements ImageAdapter using POST /images/generations.
func (a *OpenAIAdapter) Image(ctx context.Context, req ImageRequest) (ImageResponse, error) {
	n := req.N
	if n <= 0 {
		n = 1
	}
	body := openAIImageRequest{Model: req.Model, Prompt: req.Prompt, N: n, Size: req.Size}
	// dall-e models default to URL output; force base64. gpt-image-1 returns
	// base64 by default and rejects this field, so only set it for dall-e.
	if strings.HasPrefix(req.Model, "dall-e") {
		body.ResponseFormat = "b64_json"
	}

	raw, status, err := a.postJSON(ctx, "/images/generations", body)
	if err != nil {
		return ImageResponse{}, err
	}

	var parsed openAIImageResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ImageResponse{}, fmt.Errorf("%s: invalid image response (status %d): %s", a.name, status, truncate(raw))
	}
	if status >= 400 {
		return ImageResponse{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, status, errMsg(parsed.Error, status))
	}
	out := ImageResponse{}
	for _, d := range parsed.Data {
		if d.B64JSON != "" {
			out.Images = append(out.Images, d.B64JSON)
		}
	}
	if len(out.Images) == 0 {
		return ImageResponse{}, fmt.Errorf("%s: no image returned", a.name)
	}
	return out, nil
}

// --- Speech (OpenAI Audio Speech API) ---

type openAISpeechRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format,omitempty"`
}

// Speech implements SpeechAdapter using POST /audio/speech, which returns raw
// audio bytes (not JSON).
func (a *OpenAIAdapter) Speech(ctx context.Context, req SpeechRequest) (SpeechResponse, error) {
	voice := req.Voice
	if voice == "" {
		voice = "alloy"
	}
	format := req.Format
	if format == "" {
		format = "mp3"
	}

	payload, err := json.Marshal(openAISpeechRequest{
		Model:          req.Model,
		Input:          req.Input,
		Voice:          voice,
		ResponseFormat: format,
	})
	if err != nil {
		return SpeechResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return SpeechResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	for k, v := range a.extraHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return SpeechResponse{}, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return SpeechResponse{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, resp.StatusCode, truncate(data))
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = mimeForFormat(format)
	}
	return SpeechResponse{Audio: data, ContentType: contentType}, nil
}

// --- Embeddings (OpenAI Embeddings API) ---

type openAIEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Embed implements EmbeddingAdapter using POST /embeddings.
func (a *OpenAIAdapter) Embed(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error) {
	raw, status, err := a.postJSON(ctx, "/embeddings", openAIEmbeddingRequest{Model: req.Model, Input: req.Input})
	if err != nil {
		return EmbeddingResponse{}, err
	}

	var parsed openAIEmbeddingResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return EmbeddingResponse{}, fmt.Errorf("%s: invalid embedding response (status %d): %s", a.name, status, truncate(raw))
	}
	if status >= 400 {
		return EmbeddingResponse{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, status, errMsg(parsed.Error, status))
	}
	if len(parsed.Data) == 0 {
		return EmbeddingResponse{}, fmt.Errorf("%s: no embeddings returned", a.name)
	}

	// Preserve input order using the returned index.
	out := make([][]float64, len(parsed.Data))
	for _, d := range parsed.Data {
		if d.Index >= 0 && d.Index < len(out) {
			out[d.Index] = d.Embedding
		}
	}
	return EmbeddingResponse{
		Embeddings: out,
		Usage: Usage{
			PromptTokens: parsed.Usage.PromptTokens,
			TotalTokens:  parsed.Usage.TotalTokens,
		},
	}, nil
}

// postJSON is a small helper for JSON POSTs that return JSON, used by media
// endpoints. It returns the raw body and HTTP status.
func (a *OpenAIAdapter) postJSON(ctx context.Context, path string, payload any) ([]byte, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	for k, v := range a.extraHeaders {
		httpReq.Header.Set(k, v)
	}
	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, nil
}

func errMsg(e *struct {
	Message string `json:"message"`
}, status int) string {
	if e != nil && e.Message != "" {
		return e.Message
	}
	return http.StatusText(status)
}

func mimeForFormat(format string) string {
	switch format {
	case "wav":
		return "audio/wav"
	case "opus":
		return "audio/opus"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "pcm":
		return "audio/pcm"
	default:
		return "audio/mpeg"
	}
}
