package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
)

// --- Image generation (OpenAI Images API) ---

type openAIImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
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
	if a.imageAPI == "chat" {
		return a.chatImage(ctx, req, n)
	}
	body := openAIImageRequest{Model: req.Model, Prompt: req.Prompt, N: n, Size: req.Size, Quality: req.Quality}
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

// --- Image generation through chat completions (OpenRouter) ---

// chatImageAspects are the aspect ratios OpenRouter's image_config takes.
var chatImageAspects = []string{"1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9"}

// chatImageBody is the chat completion that asks an image-output model for a
// picture: the prompt and any input pictures as one user turn, modalities
// ["image","text"], and usage.include so the answer says what it cost. An
// aspect ratio is sent only when the caller asked for a shape.
func chatImageBody(req ImageRequest) map[string]any {
	content := []any{map[string]any{"type": "text", "text": req.Prompt}}
	for _, img := range req.Images {
		content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": img}})
	}
	body := map[string]any{
		"model":      req.Model,
		"messages":   []any{map[string]any{"role": "user", "content": content}},
		"modalities": []string{"image", "text"},
		"usage":      map[string]any{"include": true},
	}
	if aspect, ok := chatImageAspect(req); ok {
		body["image_config"] = map[string]any{"aspect_ratio": aspect}
	}
	return body
}

// chatImageAspect is the offered aspect ratio nearest the caller's: its
// aspect_ratio, else the shape of its WxH size.
func chatImageAspect(req ImageRequest) (string, bool) {
	w, h, ok := splitDims(strings.TrimSpace(req.AspectRatio), ":")
	if !ok {
		w, h, ok = splitDims(req.Size, "x")
	}
	if !ok {
		return "", false
	}
	want := math.Log(float64(w) / float64(h))
	best, bestDiff := "", math.Inf(1)
	for _, a := range chatImageAspects {
		aw, ah, _ := splitDims(a, ":")
		if d := math.Abs(want - math.Log(float64(aw)/float64(ah))); d < bestDiff {
			best, bestDiff = a, d
		}
	}
	return best, true
}

type chatImageResponse struct {
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
			Images  []struct {
				ImageURL struct {
					URL string `json:"url"`
				} `json:"image_url"`
			} `json:"images"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int      `json:"prompt_tokens"`
		CompletionTokens int      `json:"completion_tokens"`
		TotalTokens      int      `json:"total_tokens"`
		Cost             *float64 `json:"cost"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// chatImage generates through POST /chat/completions. Pictures come back as
// data: URIs (or, from some routes, URLs) in choices[].message.images, and
// usage.cost is what the vendor charged, in USD.
func (a *OpenAIAdapter) chatImage(ctx context.Context, req ImageRequest, n int) (ImageResponse, error) {
	raw, status, err := a.postJSON(ctx, "/chat/completions", chatImageBody(req))
	if err != nil {
		return ImageResponse{}, err
	}
	var parsed chatImageResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ImageResponse{}, fmt.Errorf("%s: invalid image response (status %d): %s", a.name, status, truncate(raw))
	}
	if status >= 400 || parsed.Error != nil {
		return ImageResponse{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, status, errMsg(parsed.Error, status))
	}
	out := ImageResponse{Usage: Usage{
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		TotalTokens:      parsed.Usage.TotalTokens,
	}}
	if parsed.Usage.Cost != nil {
		out.CostUSD, out.CostReported = *parsed.Usage.Cost, true
	}
	text := ""
	for _, c := range parsed.Choices {
		var s string
		if json.Unmarshal(c.Message.Content, &s) == nil && strings.TrimSpace(s) != "" && text == "" {
			text = strings.TrimSpace(s)
		}
		for _, img := range c.Message.Images {
			if len(out.Images) >= n {
				break
			}
			b64, err := imageDataBase64(ctx, a.name, img.ImageURL.URL)
			if err != nil {
				return ImageResponse{}, err
			}
			if b64 != "" {
				out.Images = append(out.Images, b64)
			}
		}
	}
	if len(out.Images) == 0 {
		// Still a call the vendor billed; the router does not meter a failed
		// rung, so say what came back instead.
		if text != "" {
			return ImageResponse{}, fmt.Errorf("%s: %s answered with text and no image (is it an image-output model?): %s", a.name, req.Model, truncate([]byte(text)))
		}
		return ImageResponse{}, fmt.Errorf("%s: no image returned", a.name)
	}
	return out, nil
}

// imageDataBase64 is the base64 payload of a data: URI, or the downloaded
// bytes of an http(s) URL.
func imageDataBase64(ctx context.Context, name, u string) (string, error) {
	u = strings.TrimSpace(u)
	if rest, ok := strings.CutPrefix(u, "data:"); ok {
		meta, payload, found := strings.Cut(rest, ",")
		if !found || !strings.HasSuffix(strings.ToLower(meta), ";base64") {
			return "", fmt.Errorf("%s: image data URI is not base64", name)
		}
		return payload, nil
	}
	if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
		return downloadBase64(ctx, name, u)
	}
	return "", nil
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
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions *int     `json:"dimensions,omitempty"`
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
	raw, status, err := a.postJSON(ctx, "/embeddings", openAIEmbeddingRequest{
		Model:      req.Model,
		Input:      req.Input,
		Dimensions: req.Dimensions,
	})
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

// Decide implements DecisionAdapter for OpenAI-wire providers that support
// structured decision models (e.g. OpenRouter /alpha/decisions for Jev).
func (a *OpenAIAdapter) Decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	bodyMap := map[string]any{
		"model":     req.Model,
		"state":     req.State,
		"questions": req.Questions,
	}
	body, err := json.Marshal(bodyMap)
	if err != nil {
		return DecisionResponse{}, err
	}

	url := a.baseURL + "/alpha/decisions"
	if strings.HasSuffix(a.baseURL, "/v1") {
		url = strings.TrimSuffix(a.baseURL, "/v1") + "/alpha/decisions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return DecisionResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	for k, v := range a.extraHeaders {
		httpReq.Header.Set(k, v)
	}
	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("%s: decisions request failed: %w", a.name, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return DecisionResponse{}, fmt.Errorf("%s: upstream decisions error (status %d): %s", a.name, resp.StatusCode, string(raw))
	}

	var parsed struct {
		Model   string          `json:"model"`
		Answers json.RawMessage `json:"answers"`
		Usage   struct {
			InputTokens      int `json:"input_tokens"`
			OutputTokens     int `json:"output_tokens"`
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return DecisionResponse{}, fmt.Errorf("%s: invalid decisions response: %w", a.name, err)
	}

	inp := parsed.Usage.InputTokens
	if inp == 0 {
		inp = parsed.Usage.PromptTokens
	}
	out := parsed.Usage.OutputTokens
	if out == 0 {
		out = parsed.Usage.CompletionTokens
	}

	return DecisionResponse{
		Model:   parsed.Model,
		Answers: parsed.Answers,
		Usage: Usage{
			PromptTokens:     inp,
			CompletionTokens: out,
			TotalTokens:      inp + out,
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
