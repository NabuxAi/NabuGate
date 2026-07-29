// Package nabugate is the official Go client for NabuGate, the OpenAI-compatible
// AI gateway.
//
// The gateway passes request bodies through to the upstream provider untouched,
// so requests here carry an Extra map alongside the typed fields. Anything put
// in Extra reaches the provider as-is, which means a new provider parameter
// needs no release of this package.
package nabugate

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DefaultBaseURL is the hosted gateway.
const DefaultBaseURL = "https://gate.nabuxai.com/v1"

// Client talks to a NabuGate deployment.
type Client struct {
	baseURL      string
	apiKey       string
	defaultModel string
	headers      map[string]string
	http         *http.Client
}

// Option customises a Client.
type Option func(*Client)

// WithBaseURL points the client at a different gateway deployment.
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(url, "/") }
}

// WithDefaultModel sets the alias or agent used when a request names none.
func WithDefaultModel(model string) Option {
	return func(c *Client) { c.defaultModel = model }
}

// WithHTTPClient supplies a pre-configured HTTP client (proxies, tracing).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithHeader adds a header to every request, e.g. a project identifier.
func WithHeader(key, value string) Option {
	return func(c *Client) { c.headers[key] = value }
}

// New builds a client. The only required input is the gateway API key.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		baseURL:      DefaultBaseURL,
		apiKey:       apiKey,
		defaultModel: "nabu-smart",
		headers:      map[string]string{},
		// No client-level timeout: streaming responses are long-lived by
		// design, and a blanket timeout would cut them off. Per-request
		// deadlines belong on the context the caller passes in.
		http: &http.Client{},
	}
	for _, fn := range opts {
		fn(c)
	}
	return c
}

// Error is a non-2xx response from the gateway.
type Error struct {
	StatusCode int
	Body       string
}

func (e *Error) Error() string {
	return fmt.Sprintf("nabugate: request failed (%d): %s", e.StatusCode, e.Body)
}

// Message is one chat message. Content is typed as any so multimodal parts and
// tool results pass through unchanged.
type Message struct {
	Role       string `json:"role"`
	Content    any    `json:"content"`
	Name       string `json:"name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolCalls  []any  `json:"tool_calls,omitempty"`
}

// Text builds a plain text message.
func Text(role, content string) Message { return Message{Role: role, Content: content} }

// ChatRequest is a chat completion request.
type ChatRequest struct {
	Model       string    `json:"model,omitempty"`
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
	Stop        []string  `json:"stop,omitempty"`
	Seed        *int      `json:"seed,omitempty"`
	Tools       []any     `json:"tools,omitempty"`
	ToolChoice  any       `json:"tool_choice,omitempty"`
	// ConversationID asks the gateway to replay a stored conversation.
	ConversationID string `json:"conversation_id,omitempty"`

	// Extra carries any parameter this struct does not name. It is merged into
	// the request body, so response_format, penalties and provider-specific
	// flags all work without a change here.
	Extra map[string]any `json:"-"`
}

// Usage reports token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Choice is one completion alternative.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// ChatResponse is a chat completion response.
type ChatResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Text returns the first choice's content when it is a plain string.
func (r *ChatResponse) Text() string {
	if len(r.Choices) == 0 {
		return ""
	}
	s, _ := r.Choices[0].Message.Content.(string)
	return strings.TrimSpace(s)
}

// Chat performs a chat completion.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body := c.chatBody(req, false)
	var out ChatResponse
	if err := c.postJSON(ctx, "/chat/completions", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CompleteText performs a chat completion and returns only the text.
func (c *Client) CompleteText(ctx context.Context, req ChatRequest) (string, error) {
	resp, err := c.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Text(), nil
}

// StreamChunk is one server-sent event from a streaming completion.
type StreamChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []any  `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// Stream performs a streaming chat completion, calling onChunk for each event.
// Returning an error from onChunk stops the stream and is returned to the
// caller.
func (c *Client) Stream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk) error) error {
	resp, err := c.send(ctx, http.MethodPost, "/chat/completions", c.chatBody(req, true))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Server-sent events can carry lines longer than bufio's default 64 KiB —
	// a single chunk holding a large tool-call argument does. Without this the
	// stream fails partway through with a "token too long" error.
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			return nil
		}
		var chunk StreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// Skip an unparseable payload rather than failing the whole stream.
			continue
		}
		if err := onChunk(chunk); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// StreamText is Stream reduced to text deltas.
func (c *Client) StreamText(ctx context.Context, req ChatRequest, onText func(string) error) error {
	return c.Stream(ctx, req, func(chunk StreamChunk) error {
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				if err := onText(ch.Delta.Content); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// EmbeddingsRequest asks for vectors.
type EmbeddingsRequest struct {
	Model string `json:"model,omitempty"`
	// Input is a string or a []string.
	Input any `json:"input"`
	// Dimensions pins the vector width. Set it whenever the vectors are being
	// stored: a fixed-width column cannot accept whatever the provider defaults
	// to. Leave it nil for ad-hoc search, since providers without the field
	// reject it.
	Dimensions *int `json:"dimensions,omitempty"`
}

// EmbeddingsResponse holds the vectors.
type EmbeddingsResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Usage Usage `json:"usage"`
}

// Embeddings creates embeddings.
func (c *Client) Embeddings(ctx context.Context, req EmbeddingsRequest) (*EmbeddingsResponse, error) {
	if req.Model == "" {
		req.Model = "nabu-embed"
	}
	var out EmbeddingsResponse
	if err := c.postJSON(ctx, "/embeddings", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ImageRequest asks for generated images.
type ImageRequest struct {
	Model  string `json:"model,omitempty"`
	Prompt string `json:"prompt"`
	N      int    `json:"n,omitempty"`
	Size   string `json:"size,omitempty"`
}

// ImageResponse holds the generated images, base64-encoded.
type ImageResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		B64JSON       string `json:"b64_json"`
		URL           string `json:"url"`
		RevisedPrompt string `json:"revised_prompt"`
	} `json:"data"`
}

// Images generates images.
func (c *Client) Images(ctx context.Context, req ImageRequest) (*ImageResponse, error) {
	if req.Model == "" {
		req.Model = "nabu-image"
	}
	var out ImageResponse
	if err := c.postJSON(ctx, "/images/generations", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SpeechRequest asks for synthesised speech.
type SpeechRequest struct {
	Model          string  `json:"model,omitempty"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice,omitempty"`
	ResponseFormat string  `json:"response_format,omitempty"`
	Speed          float64 `json:"speed,omitempty"`
}

// Speech synthesises speech and returns the raw audio bytes.
func (c *Client) Speech(ctx context.Context, req SpeechRequest) ([]byte, error) {
	if req.Model == "" {
		req.Model = "nabu-voice"
	}
	resp, err := c.send(ctx, http.MethodPost, "/audio/speech", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// Model describes an entry in the gateway catalogue.
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// Models lists every model, alias and agent this key may call.
func (c *Client) Models(ctx context.Context) ([]Model, error) {
	var out struct {
		Data []Model `json:"data"`
	}
	if err := c.getJSON(ctx, "/models", &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// Usage returns token and cost usage for this key.
func (c *Client) Usage(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.getJSON(ctx, "/usage", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------- internals

// chatBody merges the typed fields with Extra into one request body.
func (c *Client) chatBody(req ChatRequest, stream bool) map[string]any {
	if req.Model == "" {
		req.Model = c.defaultModel
	}
	extra := req.Extra
	req.Extra = nil

	// Round-trip through JSON so omitempty is honoured and only the fields the
	// caller actually set reach the provider.
	raw, _ := json.Marshal(req)
	body := map[string]any{}
	_ = json.Unmarshal(raw, &body)
	for k, v := range extra {
		body[k] = v
	}
	body["stream"] = stream
	return body
}

func (c *Client) send(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		// Bound the error body: a misconfigured upstream can answer with
		// megabytes of HTML, and that does not belong in an error string.
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return nil, &Error{StatusCode: resp.StatusCode, Body: string(raw)}
	}
	return resp, nil
}

func (c *Client) postJSON(ctx context.Context, path string, body, out any) error {
	resp, err := c.send(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	resp, err := c.send(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

// Float is a helper for the optional float fields.
func Float(v float64) *float64 { return &v }

// Int is a helper for the optional int fields.
func Int(v int) *int { return &v }
