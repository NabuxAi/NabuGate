package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// GeminiLiveToken is an ephemeral token for the Gemini Live API
// (BidiGenerateContent). The long-lived API key stays on the gateway; the
// caller connects with the token.
type GeminiLiveToken struct {
	// Name is the vendor's token resource, "auth_tokens/…".
	Name string
	// Expire is when the token stops working.
	Expire time.Time
}

// MintGeminiLiveToken calls POST /v1alpha/auth_tokens.
//
// setup is the BidiGenerateContent setup object and is embedded as
// bidiGenerateContentSetup without being remodelled, so tools and any other
// field the caller sent are the vendor's to read. uses is fixed at 1.
// The key is sent as x-goog-api-key, not in the URL.
func MintGeminiLiveToken(ctx context.Context, apiBase, apiKey string, setup json.RawMessage, expire, newSessionExpire time.Time) (GeminiLiveToken, error) {
	if strings.TrimSpace(apiKey) == "" {
		return GeminiLiveToken{}, fmt.Errorf("gemini live: no API key")
	}
	origin, err := geminiOrigin(apiBase)
	if err != nil {
		return GeminiLiveToken{}, err
	}
	if len(bytes.TrimSpace(setup)) == 0 {
		setup = json.RawMessage(`{}`)
	}
	if !json.Valid(setup) {
		return GeminiLiveToken{}, fmt.Errorf("gemini live: setup is not JSON")
	}
	body, err := json.Marshal(map[string]any{
		"uses":                     1,
		"expireTime":               expire.UTC().Format(time.RFC3339),
		"newSessionExpireTime":     newSessionExpire.UTC().Format(time.RFC3339),
		"bidiGenerateContentSetup": json.RawMessage(setup),
	})
	if err != nil {
		return GeminiLiveToken{}, err
	}
	endpoint := origin + "/v1alpha/auth_tokens"
	status, raw, err := postJSONOnce(ctx, endpoint, map[string]string{
		"Content-Type":   "application/json",
		"x-goog-api-key": apiKey,
	}, body)
	if err != nil {
		return GeminiLiveToken{}, fmt.Errorf("gemini live token: %w", err)
	}
	if status < 200 || status >= 300 {
		return GeminiLiveToken{}, &LiveUpstreamError{Status: status, Message: vendorErrorMessage(raw)}
	}
	var parsed struct {
		Name       string `json:"name"`
		ExpireTime string `json:"expireTime"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return GeminiLiveToken{}, fmt.Errorf("gemini live token: invalid response: %s", truncateBody(raw, 200))
	}
	if !strings.HasPrefix(parsed.Name, "auth_tokens/") {
		return GeminiLiveToken{}, fmt.Errorf("gemini live token: response has no auth_tokens name")
	}
	exp := expire
	if parsed.ExpireTime != "" {
		if t, err := time.Parse(time.RFC3339, parsed.ExpireTime); err == nil {
			exp = t
		}
	}
	return GeminiLiveToken{Name: parsed.Name, Expire: exp}, nil
}

func geminiOrigin(apiBase string) (string, error) {
	apiBase = strings.TrimSpace(apiBase)
	if apiBase == "" {
		apiBase = "https://generativelanguage.googleapis.com"
	}
	u, err := url.Parse(apiBase)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("gemini live: bad API base %q", apiBase)
	}
	return u.Scheme + "://" + u.Host, nil
}

// GeminiLiveSocketURL is the constrained BidiGenerateContent websocket the
// browser opens with an ephemeral token. access_token is the token name.
func GeminiLiveSocketURL(tokenName string) string {
	return "wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1alpha.GenerativeService.BidiGenerateContentConstrained?access_token=" + url.QueryEscape(tokenName)
}

// GeminiLiveSocketURLOn is GeminiLiveSocketURL against a chosen origin, so a
// test can point the relay at a mock websocket. origin is http(s) or ws(s).
func GeminiLiveSocketURLOn(origin, tokenName string) string {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	switch {
	case strings.HasPrefix(origin, "https://"):
		origin = "wss://" + strings.TrimPrefix(origin, "https://")
	case strings.HasPrefix(origin, "http://"):
		origin = "ws://" + strings.TrimPrefix(origin, "http://")
	}
	return origin + "/ws/google.ai.generativelanguage.v1alpha.GenerativeService.BidiGenerateContentConstrained?access_token=" + url.QueryEscape(tokenName)
}
