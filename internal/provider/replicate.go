package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ReplicateAdapter runs models on replicate.com.
//
// Replicate is not OpenAI-wire and cannot ride the openai adapter: it has no
// /chat/completions at all. Everything there is a *prediction* — you POST an
// `input` object to a model and get back a job that finishes later. So this is
// a real adapter rather than a config entry, and it implements both Chat and
// Image because on Replicate those are the same call with a different model on
// the end of it.
//
// Two shapes of model id, both addressable through the passthrough namespace
// (the router splits on the FIRST "/", so the owner/name keeps its slash):
//
//	replicate/meta/meta-llama-3-8b-instruct        → POST /models/meta/meta-llama-3-8b-instruct/predictions
//	replicate/meta/meta-llama-3-8b-instruct:5a68…  → POST /predictions  {"version": "5a68…"}
//
// The version-pinned form is the one to use when output must not change under
// you; the bare form always runs the model's current version.
//
// The prompt is read two ways, the same split GammaAdapter and ImagegenAdapter
// use, and for a stronger reason here: Replicate hosts thousands of models and
// every one publishes its own input schema. The adapter cannot know them all.
//
//   - JSON — used verbatim as the prediction's `input`. This is the escape
//     hatch for any model whose schema does not look like the common one, and
//     it is the only honest answer to a marketplace of arbitrary schemas.
//   - Plain text — mapped onto the fields nearly every Replicate text model
//     shares (prompt, system_prompt, max_tokens, temperature, top_p).
type ReplicateAdapter struct {
	name    string
	baseURL string
	apiKey  string
}

// NewReplicateAdapter builds the adapter. baseURL defaults to the public API.
func NewReplicateAdapter(name, baseURL, apiKey string) *ReplicateAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.replicate.com/v1"
	}
	return &ReplicateAdapter{name: name, baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey}
}

func (a *ReplicateAdapter) Name() string { return a.name }

// replicatePrediction is the prediction object, in the fields the gateway uses.
//
// Output is deliberately raw: it is whatever the model's output schema says,
// which across the catalogue is a string, an array of string chunks, an object,
// or a file URL. Decoding it is the caller's job, not the transport's.
type replicatePrediction struct {
	ID      string          `json:"id"`
	Status  string          `json:"status"` // starting | processing | succeeded | failed | canceled
	Output  json.RawMessage `json:"output"`
	Error   json.RawMessage `json:"error"`
	Metrics struct {
		// Token counts are model-specific on Replicate — the llama family
		// reports them, an image model has none. Absent means zero here, and a
		// zero-token record still meters the call.
		InputTokenCount  int `json:"input_token_count"`
		OutputTokenCount int `json:"output_token_count"`
	} `json:"metrics"`
	URLs struct {
		Get string `json:"get"`
	} `json:"urls"`
}

func (p replicatePrediction) done() bool {
	switch strings.ToLower(p.Status) {
	case "succeeded", "failed", "canceled":
		return true
	}
	return false
}

// errorText renders the `error` field, which is a string on most models and an
// object on a few. Either way the operator needs to read it.
func (p replicatePrediction) errorText() string {
	if len(p.Error) == 0 || string(p.Error) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(p.Error, &s) == nil {
		return s
	}
	return string(p.Error)
}

const (
	// replicateWaitSeconds asks Replicate to hold the create request open until
	// the prediction finishes. A warm text model usually returns inside it, so
	// the common case costs one round trip and no polling at all. 60 is the
	// documented maximum.
	replicateWaitSeconds  = 60
	replicatePollInterval = 2 * time.Second
	// replicatePollTimeout bounds a cold-booting or wedged prediction. Past it
	// the call fails as a timeout, which the router can fall back from —
	// instead of holding the caller's request until their own deadline.
	replicatePollTimeout = 6 * time.Minute
)

// Chat runs a text model and blocks until it has an answer.
func (a *ReplicateAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	input, err := a.chatInput(req)
	if err != nil {
		return ChatResponse{}, err
	}

	pred, err := a.run(ctx, req.Model, input)
	if err != nil {
		return ChatResponse{}, err
	}

	text, err := a.text(pred.Output, req.Model)
	if err != nil {
		return ChatResponse{}, err
	}

	usage := Usage{
		PromptTokens:     pred.Metrics.InputTokenCount,
		CompletionTokens: pred.Metrics.OutputTokenCount,
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	return ChatResponse{Content: text, FinishReason: "stop", Usage: usage}, nil
}

// chatInput builds the prediction input. A prompt that is a JSON object is used
// as the input verbatim — that is the escape hatch for a model with an unusual
// schema, and it wins over every mapping below.
func (a *ReplicateAdapter) chatInput(req ChatRequest) (map[string]any, error) {
	prompt := strings.TrimSpace(lastUserContent(req.Messages))
	if prompt == "" {
		return nil, fmt.Errorf("%s: empty prompt", a.name)
	}

	if strings.HasPrefix(prompt, "{") {
		var raw map[string]any
		if err := json.Unmarshal([]byte(prompt), &raw); err == nil && len(raw) > 0 {
			return raw, nil
		}
		// Not valid JSON after all: fall through and treat it as prose. Someone
		// writing Persian that happens to open with a brace should get an
		// answer, not a rejection.
	}

	input := map[string]any{"prompt": conversationPrompt(req.Messages)}
	if sys := systemPrompt(req.Messages); sys != "" {
		input["system_prompt"] = sys
	}
	if req.MaxTokens != nil {
		input["max_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		input["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		input["top_p"] = *req.TopP
	}
	// Replicate's text models spell stop sequences as one comma-separated
	// string. Sending it to a model that wants an array fails loudly with a 422
	// the caller can fix with the JSON escape hatch; silently dropping it would
	// let a model run past where the caller said to stop and look like the
	// model's fault.
	if stops := stopToSlice(req.Stop); len(stops) > 0 {
		input["stop_sequences"] = strings.Join(stops, ",")
	}
	return input, nil
}

// conversationPrompt renders the non-system turns into the single `prompt`
// string Replicate text models take. A one-turn request — which is most of them
// — comes out as just the user's text, with no invented role labels.
func conversationPrompt(messages []Message) string {
	var turns []Message
	for _, m := range messages {
		if m.Role != "system" && strings.TrimSpace(m.Content) != "" {
			turns = append(turns, m)
		}
	}
	if len(turns) == 1 {
		return turns[0].Content
	}
	var b strings.Builder
	for i, m := range turns {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "%s: %s", m.Role, m.Content)
	}
	if b.Len() == 0 {
		return lastUserContent(messages)
	}
	return b.String()
}

// systemPrompt joins the system turns, which Replicate takes as its own field
// rather than as part of the conversation.
func systemPrompt(messages []Message) string {
	var parts []string
	for _, m := range messages {
		if m.Role == "system" && strings.TrimSpace(m.Content) != "" {
			parts = append(parts, m.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}

// text decodes a prediction's output into the answer.
//
// Text models on Replicate stream token-by-token, so the finished output is an
// array of fragments that has to be joined with nothing between them — joining
// on a space is the classic way to get "Hel lo wor ld".
func (a *ReplicateAdapter) text(output json.RawMessage, model string) (string, error) {
	if len(output) == 0 || string(output) == "null" {
		return "", fmt.Errorf("%s: model %q returned no output", a.name, model)
	}

	var s string
	if json.Unmarshal(output, &s) == nil {
		return s, nil
	}

	var chunks []json.RawMessage
	if json.Unmarshal(output, &chunks) == nil {
		var b strings.Builder
		for _, c := range chunks {
			var part string
			if json.Unmarshal(c, &part) != nil {
				return "", a.notText(model, output)
			}
			b.WriteString(part)
		}
		return b.String(), nil
	}

	// A few models wrap the answer in an object.
	var obj struct {
		Text   string `json:"text"`
		Output string `json:"output"`
	}
	if json.Unmarshal(output, &obj) == nil {
		if obj.Text != "" {
			return obj.Text, nil
		}
		if obj.Output != "" {
			return obj.Output, nil
		}
	}

	return "", a.notText(model, output)
}

// notText names the shape that came back. A model routed to the wrong
// capability — an image model behind a chat alias — is an everyday mistake, and
// "cannot unmarshal" would send whoever hits it into the adapter instead of
// into their config.
func (a *ReplicateAdapter) notText(model string, output json.RawMessage) error {
	return fmt.Errorf("%s: model %q did not return text; its output was %s — "+
		"route it as an image alias, or send a JSON prompt matching its input schema",
		a.name, model, replicateTruncate(string(output), 200))
}

// Image runs an image model and returns the pictures base64-encoded, matching
// the gateway's image contract (data[].b64_json) so an OpenAI image client
// consumes it unchanged.
func (a *ReplicateAdapter) Image(ctx context.Context, req ImageRequest) (ImageResponse, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return ImageResponse{}, fmt.Errorf("%s: empty prompt", a.name)
	}

	input := map[string]any{"prompt": prompt}
	if strings.HasPrefix(prompt, "{") {
		var raw map[string]any
		if err := json.Unmarshal([]byte(prompt), &raw); err == nil && len(raw) > 0 {
			input = raw
		}
	}
	n := req.N
	if n < 1 {
		n = 1
	}
	if _, set := input["num_outputs"]; !set {
		input["num_outputs"] = n
	}
	if _, set := input["aspect_ratio"]; !set && req.AspectRatio != "" {
		input["aspect_ratio"] = req.AspectRatio
	}

	pred, err := a.run(ctx, req.Model, input)
	if err != nil {
		return ImageResponse{}, err
	}

	urls := imageURLs(pred.Output)
	if len(urls) == 0 {
		return ImageResponse{}, fmt.Errorf("%s: model %q returned no image URL; its output was %s",
			a.name, req.Model, replicateTruncate(string(pred.Output), 200))
	}
	if len(urls) > n {
		urls = urls[:n]
	}

	var out ImageResponse
	for _, u := range urls {
		b64, err := a.download(ctx, u)
		if err != nil {
			return ImageResponse{}, err
		}
		out.Images = append(out.Images, b64)
	}
	return out, nil
}

// imageURLs pulls the file URLs out of an output that is either one URL or an
// array of them.
func imageURLs(output json.RawMessage) []string {
	if len(output) == 0 {
		return nil
	}
	var one string
	if json.Unmarshal(output, &one) == nil {
		if strings.HasPrefix(one, "http") {
			return []string{one}
		}
		return nil
	}
	var many []string
	if json.Unmarshal(output, &many) == nil {
		var out []string
		for _, u := range many {
			if strings.HasPrefix(u, "http") {
				out = append(out, u)
			}
		}
		return out
	}
	return nil
}

// run creates a prediction and returns it finished. It asks Replicate to hold
// the request open (Prefer: wait) and only polls when that was not long enough,
// so a warm model costs exactly one round trip.
func (a *ReplicateAdapter) run(ctx context.Context, model string, input map[string]any) (replicatePrediction, error) {
	url, body, err := a.createRequest(model, input)
	if err != nil {
		return replicatePrediction{}, err
	}

	headers := a.headers()
	headers["Prefer"] = "wait=" + strconv.Itoa(replicateWaitSeconds)

	status, raw, err := postJSON(ctx, url, headers, body, a.name)
	if err != nil {
		return replicatePrediction{}, err
	}
	if status < 200 || status >= 300 {
		return replicatePrediction{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, status, replicateTruncate(string(raw), 300))
	}

	var pred replicatePrediction
	if err := json.Unmarshal(raw, &pred); err != nil {
		return replicatePrediction{}, fmt.Errorf("%s: unreadable create response: %w", a.name, err)
	}

	if !pred.done() {
		if pred, err = a.await(ctx, pred); err != nil {
			return replicatePrediction{}, err
		}
	}

	if strings.ToLower(pred.Status) != "succeeded" {
		return replicatePrediction{}, fmt.Errorf("%s: prediction %s: %s", a.name, pred.Status, replicateTruncate(pred.errorText(), 300))
	}
	return pred, nil
}

// createRequest picks the endpoint from the model id. A ":" pins a version and
// goes to /predictions; a bare "owner/name" runs the model's current version
// through /models/{owner}/{name}/predictions.
func (a *ReplicateAdapter) createRequest(model string, input map[string]any) (string, []byte, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", nil, fmt.Errorf("%s: no model", a.name)
	}

	body := map[string]any{"input": input}
	url := ""

	switch {
	case strings.Contains(model, ":"):
		// "owner/name:version" — only the version id identifies the run.
		_, version, _ := strings.Cut(model, ":")
		if strings.TrimSpace(version) == "" {
			return "", nil, fmt.Errorf("%s: model %q pins an empty version", a.name, model)
		}
		body["version"] = version
		url = a.baseURL + "/predictions"
	case strings.Contains(model, "/"):
		url = a.baseURL + "/models/" + model + "/predictions"
	default:
		// A bare version hash, which is a legitimate way to name a run.
		body["version"] = model
		url = a.baseURL + "/predictions"
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return "", nil, err
	}
	return url, raw, nil
}

// await polls the prediction to completion.
func (a *ReplicateAdapter) await(ctx context.Context, pred replicatePrediction) (replicatePrediction, error) {
	url := pred.URLs.Get
	if url == "" {
		if pred.ID == "" {
			return replicatePrediction{}, fmt.Errorf("%s: create returned neither a finished prediction nor an id", a.name)
		}
		url = a.baseURL + "/predictions/" + pred.ID
	}

	deadline := time.Now().Add(replicatePollTimeout)
	for {
		select {
		case <-ctx.Done():
			return replicatePrediction{}, ctx.Err()
		case <-time.After(replicatePollInterval):
		}

		got, err := a.poll(ctx, url)
		if err != nil {
			return replicatePrediction{}, err
		}
		if got.done() {
			return got, nil
		}
		if time.Now().After(deadline) {
			return replicatePrediction{}, fmt.Errorf("%s: prediction %s still %q after %s", a.name, got.ID, got.Status, replicatePollTimeout)
		}
	}
}

func (a *ReplicateAdapter) poll(ctx context.Context, url string) (replicatePrediction, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return replicatePrediction{}, err
	}
	for k, v := range a.headers() {
		req.Header.Set(k, v)
	}

	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return replicatePrediction{}, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return replicatePrediction{}, fmt.Errorf("%s: poll error (status %d): %s", a.name, resp.StatusCode, replicateTruncate(string(raw), 300))
	}

	var got replicatePrediction
	if err := json.Unmarshal(raw, &got); err != nil {
		return replicatePrediction{}, fmt.Errorf("%s: unreadable poll response: %w", a.name, err)
	}
	return got, nil
}

// download fetches a generated file and returns its bytes base64-encoded.
func (a *ReplicateAdapter) download(ctx context.Context, fileURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s: file download failed (status %d)", a.name, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (a *ReplicateAdapter) headers() map[string]string {
	return map[string]string{
		"Content-Type":  "application/json",
		"Accept":        "application/json",
		"Authorization": "Bearer " + a.apiKey,
	}
}

func replicateTruncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
