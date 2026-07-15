package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// openAIModelsResponse is the OpenAI GET /v1/models shape ({"data":[{"id":…}]}).
type openAIModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ListModels implements ModelLister: it queries the provider's own /v1/models
// endpoint and returns the raw upstream model IDs. The canonical OpenAI shape
// ({"data":[{"id":…}]}) is preferred; a bare array of objects or of strings —
// which some OpenAI-wire aggregators return — is accepted as a fallback.
func (a *OpenAIAdapter) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	for k, v := range a.headers() {
		req.Header.Set(k, v)
	}
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg := http.StatusText(resp.StatusCode)
		var parsed openAIModelsResponse
		if json.Unmarshal(raw, &parsed) == nil && parsed.Error != nil && parsed.Error.Message != "" {
			msg = parsed.Error.Message
		}
		return nil, fmt.Errorf("%s: models error (status %d): %s", a.name, resp.StatusCode, msg)
	}

	// Preferred shape: {"data":[{"id":…}]}.
	var parsed openAIModelsResponse
	if json.Unmarshal(raw, &parsed) == nil && len(parsed.Data) > 0 {
		out := make([]string, 0, len(parsed.Data))
		for _, m := range parsed.Data {
			if m.ID != "" {
				out = append(out, m.ID)
			}
		}
		return out, nil
	}
	// Fallback: a bare array of {"id":…} objects.
	var bareObjs []struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &bareObjs) == nil && len(bareObjs) > 0 {
		out := make([]string, 0, len(bareObjs))
		for _, m := range bareObjs {
			if m.ID != "" {
				out = append(out, m.ID)
			}
		}
		return out, nil
	}
	// Fallback: a bare array of plain strings.
	var bareStrs []string
	if json.Unmarshal(raw, &bareStrs) == nil {
		out := make([]string, 0, len(bareStrs))
		for _, s := range bareStrs {
			if s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out, nil
		}
	}
	return nil, fmt.Errorf("%s: could not parse /models response: %s", a.name, truncate(raw))
}

// Responses implements ResponsesAdapter: it forwards the (already
// model-rewritten) Responses API body to the provider's /v1/responses endpoint
// and returns the raw upstream response for the caller to stream back. The
// streaming HTTP client is used so a `"stream": true` SSE response is not cut
// off by a whole-request timeout; the call is bounded by ctx instead.
func (a *OpenAIAdapter) Responses(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range a.headers() {
		req.Header.Set(k, v)
	}
	return streamHTTPClient.Do(req)
}
