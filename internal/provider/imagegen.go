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
)

// ImagegenAdapter renders branded graphics through mrc_imagegen and serves them
// as an ImageAdapter, so a caller asks for an image the same way it would ask
// OpenAI and gets back base64 PNG.
//
// The thing to understand before using it: mrc_imagegen is not a text-to-image
// model. It is a template renderer. It takes a kicker, two headline lines and a
// theme, and lays them out on a fixed canvas with the brand's typography and
// palette. Handing it a paragraph of free prose produces a headline that
// overflows its box, not a picture of what the prose describes.
//
// So the prompt is read two ways:
//
//   - JSON with the renderer's own fields — full control, deterministic, no
//     guessing. This is what a caller that already knows its copy should send.
//   - Plain text — split on the first line break or sentence boundary into
//     head1/head2 and trimmed to what the layout can hold.
//
// The plain-text path is a convenience, not intelligence. A caller that wants
// well-composed Persian copy should get it from the mrc-imagegen-writer agent —
// which is one chat call it is usually making anyway — and pass the result here
// as JSON. Doing that inside this adapter would mean an image request silently
// failing for a text-model reason, which is a bad trade for a rendering path
// that is otherwise instant and deterministic.
type ImagegenAdapter struct {
	name    string
	baseURL string
	apiKey  string
}

// NewImagegenAdapter builds the adapter. baseURL is the mrc_imagegen service
// root, e.g. https://imagen-api.nabuxai.com.
func NewImagegenAdapter(name, baseURL, apiKey string) *ImagegenAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://imagen-api.nabuxai.com"
	}
	return &ImagegenAdapter{name: name, baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey}
}

func (a *ImagegenAdapter) Name() string { return a.name }

// Chat is unsupported. It exists so the adapter satisfies the base Adapter
// interface and can sit in the router's adapter map; routing chat here (a
// misconfigured alias) fails cleanly instead of panicking.
func (a *ImagegenAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, fmt.Errorf("%s: chat is not supported by the imagegen provider", a.name)
}

// renderRequest mirrors mrc_imagegen's POST /v1/render body. Only the fields
// the gateway can meaningfully populate are here; the service defaults the rest.
type renderRequest struct {
	Kind    string `json:"kind"`
	Kicker  string `json:"kicker,omitempty"`
	Head1   string `json:"head1,omitempty"`
	Head2   string `json:"head2,omitempty"`
	Subtext string `json:"subtext,omitempty"`
	CTA     string `json:"cta,omitempty"`
	Theme   string `json:"theme,omitempty"`
	Brand   string `json:"brand,omitempty"`
	Palette any    `json:"palette,omitempty"`
	Variant int    `json:"variant,omitempty"`
	Design  int    `json:"design,omitempty"`

	PhotoURL   string `json:"photo_url,omitempty"`
	PhotoQuery string `json:"photo_query,omitempty"`

	Format string  `json:"format"`
	Scale  float64 `json:"scale,omitempty"`
}

// promptFields is the JSON a caller may send in the prompt to drive the render
// directly. It is deliberately the same shape as the render body, minus the
// transport concerns (format and scale), which the gateway decides.
type promptFields struct {
	Kind    string `json:"kind"`
	Kicker  string `json:"kicker"`
	Head1   string `json:"head1"`
	Head2   string `json:"head2"`
	Subtext string `json:"subtext"`
	CTA     string `json:"cta"`
	Theme   string `json:"theme"`
	Brand   string `json:"brand"`
	Palette any    `json:"palette"`
	Variant int    `json:"variant"`
	Design  int    `json:"design"`

	PhotoURL   string `json:"photo_url"`
	PhotoQuery string `json:"photo_query"`
}

// Layout limits, mirroring the renderer's canvas. Text past these does not wrap
// gracefully — it collides with the frame — so it is cut here where the reason
// can be explained, rather than silently ruining the image.
const (
	maxKicker  = 40
	maxHeadLn  = 60
	maxSubtext = 200
)

// Image renders one graphic per requested image.
func (a *ImagegenAdapter) Image(ctx context.Context, req ImageRequest) (ImageResponse, error) {
	n := req.N
	if n < 1 {
		n = 1
	}
	if n > 4 {
		// Each image is a full render; a large n turns one API call into a long
		// one. Callers that want a set should ask in a loop and see progress.
		n = 4
	}

	base := a.buildRequest(req)

	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		body := base
		// Vary the layout across a batch so "give me 3" is three designs of the
		// same copy rather than the same file three times.
		if n > 1 {
			switch body.Kind {
			case "header":
				body.Variant = base.Variant + i
			case "design":
				body.Design = base.Design + i
			}
		}
		img, err := a.render(ctx, body)
		if err != nil {
			if len(out) > 0 {
				// Partial success beats none: return what rendered and let the
				// caller decide, the same way the router treats a short batch.
				break
			}
			return ImageResponse{}, err
		}
		out = append(out, img)
	}

	return ImageResponse{Images: out}, nil
}

// buildRequest turns an OpenAI-style image request into a render body.
func (a *ImagegenAdapter) buildRequest(req ImageRequest) renderRequest {
	body := renderRequest{
		Kind:   kindFromModel(req.Model, req.AspectRatio, req.Size),
		Format: "png",
		Scale:  2,
	}

	prompt := strings.TrimSpace(req.Prompt)

	// JSON first: a caller that knows its copy says so exactly.
	if strings.HasPrefix(prompt, "{") {
		var f promptFields
		if err := json.Unmarshal([]byte(prompt), &f); err == nil {
			if f.Kind != "" {
				body.Kind = f.Kind
			}
			body.Kicker = clip(f.Kicker, maxKicker)
			body.Head1 = clip(f.Head1, maxHeadLn)
			body.Head2 = clip(f.Head2, maxHeadLn)
			body.Subtext = clip(f.Subtext, maxSubtext)
			body.CTA = f.CTA
			body.Theme = f.Theme
			body.Brand = f.Brand
			body.Palette = f.Palette
			body.Variant = f.Variant
			body.Design = f.Design
			body.PhotoURL = f.PhotoURL
			body.PhotoQuery = f.PhotoQuery
			return body
		}
		// Malformed JSON falls through to the text path rather than erroring:
		// a caller who meant to send prose that happens to start with a brace
		// still gets an image.
	}

	head1, head2 := splitHeadline(prompt)
	body.Head1 = head1
	body.Head2 = head2
	return body
}

// kindFromModel picks the canvas. The model name carries it — "nabu-card",
// "nabu-header", "nabu-story" — because that is the one field an OpenAI image
// client always controls. Falling back to the aspect ratio or size covers a
// caller that only set those.
func kindFromModel(model, aspect, size string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "card"):
		return "card"
	case strings.Contains(m, "story"):
		return "story"
	case strings.Contains(m, "design"):
		return "design"
	case strings.Contains(m, "header"):
		return "header"
	}

	// 1080x1350 portrait is the card; everything else defaults to the 16:9
	// header, which is what an article or a blog post wants.
	if aspect == "4:5" || aspect == "3:4" {
		return "card"
	}
	if aspect == "9:16" {
		return "story"
	}
	if w, h, ok := parseSize(size); ok && h > w {
		return "card"
	}
	return "header"
}

func parseSize(size string) (int, int, bool) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return w, h, true
}

// splitHeadline turns a line of prose into the renderer's two headline lines.
//
// Break on an explicit newline if the caller gave one — that is them saying
// where the line ends. Otherwise split near the middle on a word boundary,
// which is what a designer does by eye and what keeps both lines a similar
// length. A short prompt stays on one line rather than being forced into two.
func splitHeadline(s string) (string, string) {
	// Look for the caller's own line break before normalising whitespace —
	// strings.Fields() would turn it into an ordinary space and the break would
	// be lost.
	if i := strings.IndexByte(s, '\n'); i > 0 {
		first := strings.Join(strings.Fields(s[:i]), " ")
		rest := strings.Join(strings.Fields(s[i+1:]), " ")
		if first != "" {
			return clip(first, maxHeadLn), clip(rest, maxHeadLn)
		}
	}

	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "", ""
	}

	r := []rune(s)
	if len(r) <= maxHeadLn/2 {
		return string(r), ""
	}

	mid := len(r) / 2
	best := -1
	for i := range r {
		if r[i] != ' ' {
			continue
		}
		if best == -1 || abs(i-mid) < abs(best-mid) {
			best = i
		}
	}
	if best <= 0 {
		return clip(s, maxHeadLn), ""
	}
	return clip(string(r[:best]), maxHeadLn), clip(strings.TrimSpace(string(r[best:])), maxHeadLn)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// clip trims to a rune budget on a word boundary where it can, so a cut lands
// between words instead of mid-syllable. Persian is counted in runes, not
// bytes — a byte limit would cut a character in half.
func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := string(r[:max])
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut)
}

// render performs one call and returns the image base64-encoded.
func (a *ImagegenAdapter) render(ctx context.Context, body renderRequest) (string, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("%s: encode request: %w", a.name, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/render", strings.NewReader(string(raw)))
	if err != nil {
		return "", fmt.Errorf("%s: build request: %w", a.name, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "image/png")
	// mrc_imagegen authenticates with X-API-Key, not a bearer token.
	httpReq.Header.Set("X-API-Key", a.apiKey)

	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%s: render: %w", a.name, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return "", fmt.Errorf("%s: read render response: %w", a.name, err)
	}

	if resp.StatusCode != http.StatusOK {
		// The service answers errors as JSON and successes as image bytes, so
		// surface the message rather than a status code on its own.
		msg := strings.TrimSpace(string(data))
		var e struct {
			Detail any `json:"detail"`
		}
		if json.Unmarshal(data, &e) == nil && e.Detail != nil {
			msg = fmt.Sprintf("%v", e.Detail)
		}
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return "", fmt.Errorf("%s: render failed (%d): %s", a.name, resp.StatusCode, msg)
	}

	if len(data) == 0 {
		// A 200 with no body would otherwise become a valid-looking empty
		// image, which the caller has no way to distinguish from a blank design.
		return "", fmt.Errorf("%s: render returned an empty image", a.name)
	}

	return base64.StdEncoding.EncodeToString(data), nil
}
