package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// PexelsAdapter serves photographs from the Pexels stock-photo API as an
// ImageAdapter, so an image alias can return real photos instead of (or as a
// fallback for) AI-generated images — and projects reach Pexels through the
// gateway rather than calling it directly.
//
// Pexels is not OpenAI-wire: it is a search API returning hosted photo URLs. So
// this adapter searches for the prompt (or pulls the curated feed when the
// prompt is empty), downloads the chosen photos, and returns their bytes
// base64-encoded — matching the gateway's image response contract
// (data[].b64_json), so any OpenAI image client consumes it unchanged.
type PexelsAdapter struct {
	name    string
	baseURL string
	apiKey  string
}

// NewPexelsAdapter builds a Pexels image adapter. baseURL defaults to the public
// API root when empty.
func NewPexelsAdapter(name, baseURL, apiKey string) *PexelsAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.pexels.com/v1"
	}
	return &PexelsAdapter{name: name, baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey}
}

func (a *PexelsAdapter) Name() string { return a.name }

// Chat is unsupported: Pexels is an image-only provider. It exists so the
// adapter satisfies the base Adapter interface and can live in the router's
// adapter map; only image aliases are ever routed here, and the router skips a
// provider that lacks a needed capability. Routing chat here (a misconfigured
// alias) fails cleanly rather than panicking.
func (a *PexelsAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, fmt.Errorf("%s: chat is not supported by the Pexels provider", a.name)
}

type pexelsSearchResponse struct {
	Photos []struct {
		ID           int    `json:"id"`
		URL          string `json:"url"`
		Photographer string `json:"photographer"`
		Src          struct {
			Original string `json:"original"`
			Large2x  string `json:"large2x"`
			Large    string `json:"large"`
			Medium   string `json:"medium"`
		} `json:"src"`
	} `json:"photos"`
	Error string `json:"error"`
}

// Image implements ImageAdapter. It searches Pexels for req.Prompt (or pulls the
// curated feed when the prompt is empty), then downloads up to req.N photos and
// returns them base64-encoded.
func (a *PexelsAdapter) Image(ctx context.Context, req ImageRequest) (ImageResponse, error) {
	n := req.N
	if n <= 0 {
		n = 1
	}
	perPage := n
	if perPage < 15 {
		perPage = 15 // fetch a small pool so N distinct photos are available
	}
	if perPage > 80 {
		perPage = 80 // Pexels caps per_page at 80
	}

	q := url.Values{}
	q.Set("per_page", strconv.Itoa(perPage))
	endpoint := "/curated"
	if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
		endpoint = "/search"
		q.Set("query", prompt)
		if o := pexelsOrientation(req); o != "" {
			q.Set("orientation", o)
		}
	}

	raw, status, err := a.get(ctx, endpoint+"?"+q.Encode())
	if err != nil {
		return ImageResponse{}, err
	}
	var parsed pexelsSearchResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ImageResponse{}, fmt.Errorf("%s: invalid response (status %d): %s", a.name, status, truncate(raw))
	}
	if status >= 400 {
		msg := parsed.Error
		if msg == "" {
			msg = http.StatusText(status)
		}
		return ImageResponse{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, status, msg)
	}
	if len(parsed.Photos) == 0 {
		return ImageResponse{}, fmt.Errorf("%s: no photos for %q", a.name, req.Prompt)
	}

	out := ImageResponse{}
	for _, p := range parsed.Photos {
		if len(out.Images) >= n {
			break
		}
		src := firstNonEmpty(p.Src.Large2x, p.Src.Large, p.Src.Original, p.Src.Medium)
		if src == "" {
			continue
		}
		b64, err := a.download(ctx, src)
		if err != nil {
			return ImageResponse{}, err
		}
		out.Images = append(out.Images, b64)
	}
	if len(out.Images) == 0 {
		return ImageResponse{}, fmt.Errorf("%s: no downloadable photos", a.name)
	}
	return out, nil
}

// pexelsOrientation maps an OpenAI-style aspect ratio / size to a Pexels
// orientation, best-effort. Returns "" when it cannot be determined.
func pexelsOrientation(req ImageRequest) string {
	w, h := 0, 0
	if a, b, ok := splitDims(req.AspectRatio, ":"); ok {
		w, h = a, b
	} else if a, b, ok := splitDims(req.Size, "x"); ok {
		w, h = a, b
	}
	switch {
	case w == 0 || h == 0:
		return ""
	case w > h:
		return "landscape"
	case h > w:
		return "portrait"
	default:
		return "square"
	}
}

func splitDims(s, sep string) (int, int, bool) {
	parts := strings.SplitN(strings.TrimSpace(s), sep, 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	a, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	b, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || a <= 0 || b <= 0 {
		return 0, 0, false
	}
	return a, b, true
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// get issues an authenticated GET to the Pexels API. Pexels puts the raw key in
// the Authorization header (no "Bearer" prefix).
func (a *PexelsAdapter) get(ctx context.Context, path string) ([]byte, int, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Authorization", a.apiKey)
	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, nil
}

// download fetches an image URL and returns its bytes base64-encoded.
func (a *PexelsAdapter) download(ctx context.Context, imageURL string) (string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s: image download failed (status %d)", a.name, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
