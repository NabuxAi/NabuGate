package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GammaAdapter generates decks, documents and social posts through gamma.app
// and serves them as a chat model: the caller sends a prompt, the adapter
// returns links to the finished gamma and, when one was requested, its export.
//
// It is a chat adapter rather than an ImageAdapter for two reasons. Gamma
// produces a multi-card artefact hosted on gamma.app — a URL, not pixels — so
// ImageResponse (base64 PNG) has nowhere to put the result. And the API is
// asynchronous: POST /generations answers with an id, and the work finishes
// somewhere between a few seconds and a couple of minutes later. Chat is the
// only shape here that can hold a request open that long and hand back text.
//
// The prompt is read two ways, the same split ImagegenAdapter uses:
//
//   - JSON with Gamma's own fields — full control over format, card count,
//     theme, language and export type. This is what a caller that already knows
//     what it wants should send.
//   - Plain text — used as inputText with the format taken from the model name
//     (gamma-presentation | gamma-document | gamma-social), which is the
//     everyday path.
//
// Nothing here is inferred about the content. Gamma writes the copy itself from
// inputText, so an adapter that rewrote the prompt would be arguing with the
// service it is calling.
type GammaAdapter struct {
	name    string
	baseURL string
	apiKey  string
}

// NewGammaAdapter builds the adapter. baseURL is the Gamma public API root.
func NewGammaAdapter(name, baseURL, apiKey string) *GammaAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://public-api.gamma.app"
	}
	return &GammaAdapter{name: name, baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey}
}

func (a *GammaAdapter) Name() string { return a.name }

// gammaRequest mirrors POST /v0.2/generations. Only the fields the gateway can
// meaningfully populate are here; Gamma defaults the rest.
type gammaRequest struct {
	InputText              string         `json:"inputText"`
	TextMode               string         `json:"textMode,omitempty"` // generate | condense | preserve
	Format                 string         `json:"format,omitempty"`   // presentation | document | social
	ThemeName              string         `json:"themeName,omitempty"`
	NumCards               int            `json:"numCards,omitempty"`
	CardSplit              string         `json:"cardSplit,omitempty"` // auto | inputTextBreaks
	AdditionalInstructions string         `json:"additionalInstructions,omitempty"`
	ExportAs               string         `json:"exportAs,omitempty"` // pdf | pptx
	TextOptions            map[string]any `json:"textOptions,omitempty"`
	ImageOptions           map[string]any `json:"imageOptions,omitempty"`
	CardOptions            map[string]any `json:"cardOptions,omitempty"`
	SharingOptions         map[string]any `json:"sharingOptions,omitempty"`
}

type gammaCreateResponse struct {
	GenerationID string `json:"generationId"`
}

type gammaStatusResponse struct {
	Status    string `json:"status"` // pending | completed | failed
	GammaURL  string `json:"gammaUrl"`
	ExportURL string `json:"exportUrl"`
	Credits   struct {
		Deducted  int `json:"deducted"`
		Remaining int `json:"remaining"`
	} `json:"credits"`
	Message string `json:"message"`
}

// Poll cadence. Gamma finishes a short deck in well under a minute and a long
// one in a few; the ceiling exists so a stuck generation fails as a timeout
// rather than holding a request open until the caller's own deadline.
const (
	gammaPollInterval = 3 * time.Second
	gammaPollTimeout  = 5 * time.Minute
)

// Chat generates the gamma and blocks until it is ready.
func (a *GammaAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	prompt := lastUserContent(req.Messages)
	if strings.TrimSpace(prompt) == "" {
		return ChatResponse{}, fmt.Errorf("%s: empty prompt", a.name)
	}

	body := a.buildRequest(req.Model, prompt)
	if strings.TrimSpace(body.InputText) == "" {
		return ChatResponse{}, fmt.Errorf("%s: inputText is required", a.name)
	}

	id, err := a.create(ctx, body)
	if err != nil {
		return ChatResponse{}, err
	}

	status, err := a.await(ctx, id)
	if err != nil {
		return ChatResponse{}, err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[%s](%s)", gammaTitle(body.Format), status.GammaURL)
	if status.ExportURL != "" {
		fmt.Fprintf(&b, "\n\n[%s](%s)", strings.ToUpper(body.ExportAs), status.ExportURL)
	}
	if status.Credits.Remaining > 0 {
		fmt.Fprintf(&b, "\n\ncredits: %d used, %d remaining", status.Credits.Deducted, status.Credits.Remaining)
	}

	return ChatResponse{Content: b.String(), FinishReason: "stop"}, nil
}

// buildRequest reads the prompt as Gamma's own JSON when it is JSON, and as
// inputText otherwise. An unparseable body is not an error: a caller writing
// Persian prose that happens to start with a brace should get a deck, not a
// rejection.
func (a *GammaAdapter) buildRequest(model, prompt string) gammaRequest {
	out := gammaRequest{Format: formatFromModel(model)}

	trimmed := strings.TrimSpace(prompt)
	if strings.HasPrefix(trimmed, "{") {
		var parsed gammaRequest
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil && strings.TrimSpace(parsed.InputText) != "" {
			if parsed.Format == "" {
				parsed.Format = out.Format
			}
			return parsed
		}
	}

	out.InputText = trimmed
	return out
}

// formatFromModel maps the alias's upstream model name onto Gamma's format.
// Presentation is the default because it is what "make me slides" means.
func formatFromModel(model string) string {
	switch {
	case strings.Contains(model, "document"):
		return "document"
	case strings.Contains(model, "social"):
		return "social"
	default:
		return "presentation"
	}
}

func gammaTitle(format string) string {
	switch format {
	case "document":
		return "Gamma document"
	case "social":
		return "Gamma social post"
	default:
		return "Gamma presentation"
	}
}

func (a *GammaAdapter) create(ctx context.Context, body gammaRequest) (string, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	status, resp, err := postJSON(ctx, a.baseURL+"/v0.2/generations", a.headers(), raw, a.name)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("%s: upstream error (status %d): %s", a.name, status, gammaTruncate(string(resp), 300))
	}

	var created gammaCreateResponse
	if err := json.Unmarshal(resp, &created); err != nil {
		return "", fmt.Errorf("%s: unreadable create response: %w", a.name, err)
	}
	if created.GenerationID == "" {
		return "", fmt.Errorf("%s: create returned no generationId", a.name)
	}
	return created.GenerationID, nil
}

// await polls until the generation completes. A failed generation is an error
// rather than a link to nothing, and so is a timeout.
func (a *GammaAdapter) await(ctx context.Context, id string) (gammaStatusResponse, error) {
	deadline := time.Now().Add(gammaPollTimeout)

	for {
		select {
		case <-ctx.Done():
			return gammaStatusResponse{}, ctx.Err()
		case <-time.After(gammaPollInterval):
		}

		got, err := a.poll(ctx, id)
		if err != nil {
			return gammaStatusResponse{}, err
		}

		switch strings.ToLower(got.Status) {
		case "completed":
			if got.GammaURL == "" {
				return gammaStatusResponse{}, fmt.Errorf("%s: generation completed with no gammaUrl", a.name)
			}
			return got, nil
		case "failed", "error":
			return gammaStatusResponse{}, fmt.Errorf("%s: generation failed: %s", a.name, gammaTruncate(got.Message, 300))
		}

		if time.Now().After(deadline) {
			return gammaStatusResponse{}, fmt.Errorf("%s: generation %s still %q after %s", a.name, id, got.Status, gammaPollTimeout)
		}
	}
}

func (a *GammaAdapter) poll(ctx context.Context, id string) (gammaStatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/v0.2/generations/"+id, nil)
	if err != nil {
		return gammaStatusResponse{}, err
	}
	for k, v := range a.headers() {
		req.Header.Set(k, v)
	}

	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return gammaStatusResponse{}, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return gammaStatusResponse{}, fmt.Errorf("%s: poll error (status %d): %s", a.name, resp.StatusCode, gammaTruncate(string(raw), 300))
	}

	var got gammaStatusResponse
	if err := json.Unmarshal(raw, &got); err != nil {
		return gammaStatusResponse{}, fmt.Errorf("%s: unreadable poll response: %w", a.name, err)
	}
	return got, nil
}

func (a *GammaAdapter) headers() map[string]string {
	return map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
		"X-API-KEY":    a.apiKey,
	}
}

func lastUserContent(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	if len(messages) > 0 {
		return messages[len(messages)-1].Content
	}
	return ""
}

func gammaTruncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
