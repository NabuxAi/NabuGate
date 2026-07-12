// Package provider defines the common request/response shapes used across all
// upstream LLM providers and the Adapter interface each provider implements.
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Message is a single chat message in the unified (OpenAI-style) format.
type Message struct {
	Role    string `json:"role"`    // "system" | "user" | "assistant"
	Content string `json:"content"` // plain text content
}

// ChatRequest is the provider-agnostic chat request. Model is the *upstream*
// model name (already resolved from an alias by the router).
type ChatRequest struct {
	Model       string
	Messages    []Message
	Temperature *float64
	TopP        *float64
	MaxTokens   *int
	Stop        json.RawMessage // OpenAI "stop": string or []string

	// Raw is the original OpenAI-style request body, field by field. OpenAI-wire
	// adapters forward it verbatim (overriding only model/stream) so any
	// parameter — tools, tool_choice, response_format, seed, penalties, … —
	// passes through untouched. Non-OpenAI adapters (Anthropic, Gemini) translate
	// the typed fields above instead.
	Raw map[string]json.RawMessage
}

// Usage captures token accounting returned by the upstream provider.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResponse is the normalized response returned by every adapter.
type ChatResponse struct {
	Content      string
	ToolCalls    json.RawMessage // raw OpenAI tool_calls array, if the model called tools
	FinishReason string          // normalized: "stop" | "length" | "tool_calls"
	Usage        Usage
}

// Adapter converts the unified request into a provider's native API call and
// normalizes the response back. One Adapter instance maps to one configured
// provider (e.g. "openai", "groq", "anthropic").
type Adapter interface {
	// Name returns the configured provider name.
	Name() string
	// Chat performs a single non-streaming chat completion.
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

// DeltaFunc receives incremental text chunks during a streaming completion.
type DeltaFunc func(delta string) error

// StreamAdapter is implemented by providers that can stream chat completions.
// onDelta is called for each text chunk; the returned Usage carries final token
// accounting when the provider reports it.
type StreamAdapter interface {
	ChatStream(ctx context.Context, req ChatRequest, onDelta DeltaFunc) (Usage, error)
}

// ImageRequest is a provider-agnostic image generation request.
type ImageRequest struct {
	Model       string
	Prompt      string
	N           int    // number of images (default 1)
	AspectRatio string // e.g. "16:9" (best-effort; not all providers honor it)
	Size        string // e.g. "1024x1024" (OpenAI-style)
}

// ImageResponse carries one or more base64-encoded PNG images.
type ImageResponse struct {
	Images []string // base64 PNG data (no data: prefix)
}

// ImageAdapter is implemented by providers that can generate images.
type ImageAdapter interface {
	Image(ctx context.Context, req ImageRequest) (ImageResponse, error)
}

// SpeechRequest is a provider-agnostic text-to-speech request.
type SpeechRequest struct {
	Model  string
	Input  string
	Voice  string
	Format string // requested container, e.g. "mp3" | "wav" (best-effort)
}

// SpeechResponse carries a ready-to-play audio file.
type SpeechResponse struct {
	Audio       []byte
	ContentType string // e.g. "audio/wav" | "audio/mpeg"
}

// SpeechAdapter is implemented by providers that can synthesize speech.
type SpeechAdapter interface {
	Speech(ctx context.Context, req SpeechRequest) (SpeechResponse, error)
}

// EmbeddingRequest is a provider-agnostic text embedding request.
type EmbeddingRequest struct {
	Model string
	Input []string
}

// EmbeddingResponse carries one vector per input (same order).
type EmbeddingResponse struct {
	Embeddings [][]float64
	Usage      Usage
}

// EmbeddingAdapter is implemented by providers that can embed text.
type EmbeddingAdapter interface {
	Embed(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error)
}

// userAgent is the product User-Agent sent on every outbound provider request.
// Go's net/http default ("Go-http-client/1.1") is treated as a bot signature by
// some upstream WAFs (e.g. Parspack AI Studio), which then answer with a 403
// block page instead of the API response, so we always identify as the gateway.
const userAgent = "NabuGate/1.0"

// userAgentTransport injects userAgent on any request that does not already set
// a User-Agent, then delegates to base. It wraps the transport of both shared
// clients so every adapter/endpoint (chat, stream, image, speech, embeddings)
// is covered in one place.
type userAgentTransport struct {
	base http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		// Clone before mutating: a RoundTripper must not modify the caller's request.
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", userAgent)
	}
	return t.base.RoundTrip(req)
}

// sharedHTTPClient is reused by all adapters for non-streaming calls; each call
// is also bounded by the request context, so the client timeout is a generous
// safety net.
var sharedHTTPClient = &http.Client{
	Timeout:   120 * time.Second,
	Transport: &userAgentTransport{base: http.DefaultTransport},
}

// streamHTTPClient is used for SSE streaming. It deliberately has NO
// whole-request timeout: http.Client.Timeout also covers reading the response
// body, so a fixed cap would sever a long-running stream mid-generation.
// Streaming is instead bounded by the request context (client disconnect or
// server shutdown), while ResponseHeaderTimeout still caps a dead upstream that
// never sends the first byte.
var streamHTTPClient = newStreamClient()

func newStreamClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ResponseHeaderTimeout = 120 * time.Second
	return &http.Client{Transport: &userAgentTransport{base: tr}}
}

// isTransient reports whether an upstream HTTP status is worth retrying.
func isTransient(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// postJSON sends a JSON POST and retries transient failures (network errors,
// 429, 5xx) with exponential backoff, bounded by ctx. It returns the final
// status code and response body. The body is fixed across attempts, so it is
// safe to replay.
func postJSON(ctx context.Context, url string, headers map[string]string, body []byte, name string) (int, []byte, error) {
	const maxAttempts = 3
	backoff := 200 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return 0, nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return 0, nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := sharedHTTPClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if isTransient(resp.StatusCode) {
			lastErr = fmt.Errorf("%s: transient upstream status %d", name, resp.StatusCode)
			continue
		}
		return resp.StatusCode, raw, nil
	}
	return 0, nil, lastErr
}

// stopToSlice normalizes an OpenAI "stop" value (string or []string) to a slice,
// used by the non-OpenAI adapters.
func stopToSlice(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return []string{s}
	}
	return nil
}
