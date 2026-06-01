// Package provider defines the common request/response shapes used across all
// upstream LLM providers and the Adapter interface each provider implements.
package provider

import (
	"context"
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
	MaxTokens   *int
}

// Usage captures token accounting returned by the upstream provider.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResponse is the normalized response returned by every adapter.
type ChatResponse struct {
	Content string
	Usage   Usage
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

// sharedHTTPClient is reused by all adapters; upstream calls are bounded by the
// request context, so the client timeout is a generous safety net.
var sharedHTTPClient = &http.Client{Timeout: 120 * time.Second}
