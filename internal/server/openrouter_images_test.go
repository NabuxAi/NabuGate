package server

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"nabugate/internal/agent"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// openRouterVendor answers /chat/completions like OpenRouter's image route
// and remembers the body it was sent.
func openRouterVendor(t *testing.T, answer string, sent *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		if sent != nil {
			_ = json.Unmarshal(raw, sent)
		}
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// openRouterImageServer is the gateway with OpenRouter as a passthrough
// provider on the chat image route, and NabuGen's allow-list as shipped.
func openRouterImageServer(t *testing.T, vendorURL string, prices map[string]usage.Price) (*httptest.Server, *usage.Tracker) {
	t.Helper()
	or := provider.NewOpenAIAdapter("openrouter", vendorURL, "k", nil)
	if err := or.SetImageAPI("chat"); err != nil {
		t.Fatal(err)
	}
	r := router.New(map[string]provider.Adapter{"openrouter": or}, nil, nil, nil, nil, nil,
		map[string][]string{"openrouter": nil}, discardLogger())
	keys := policy.New(nil, []policy.KeyConfig{
		{Key: "gen-key", Project: "nabugen", Allow: []string{"nabu-*", "gen-*", "openrouter/*", "elevenlabs"}, RateLimit: 10000},
		{Key: "chat-only", Project: "other", Allow: []string{"nabu-*"}, RateLimit: 10000},
	})
	tracker := usage.New(prices)
	ts := httptest.NewServer(New(r, keys, tracker, agent.NewRegistry(), discardLogger()).Handler())
	t.Cleanup(ts.Close)
	return ts, tracker
}

func postImagesAs(t *testing.T, ts *httptest.Server, key, body string) (*http.Response, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/images/generations", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]any{"raw": string(raw)}
	}
	return resp, out
}

// NabuGen's key reaches an OpenRouter image model by its passthrough id, with
// an input picture for an edit; the picture comes back as b64_json and the
// call is metered at the cost OpenRouter reported, not a price-table guess.
func TestOpenRouterImageEditIsServedAndMeteredAtTheReportedCost(t *testing.T) {
	var sent map[string]any
	answer := `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"Done.",` +
		`"images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + base64.StdEncoding.EncodeToString([]byte("watercolor")) + `"}}]}}],` +
		`"usage":{"prompt_tokens":270,"completion_tokens":1290,"total_tokens":1560,"cost":0.0391}}`
	// A table price that must NOT be used when the vendor said what it cost.
	ts, tracker := openRouterImageServer(t, openRouterVendor(t, answer, &sent).URL,
		map[string]usage.Price{"gemini-2.5-flash-image": {PerImage: 0.5}})

	resp, out := postImagesAs(t, ts, "gen-key",
		`{"model":"openrouter/google/gemini-2.5-flash-image","prompt":"make it a watercolor","images":["https://example.com/a.png"],"aspect_ratio":"4:3"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("answered %d: %v", resp.StatusCode, out)
	}
	data, _ := out["data"].([]any)
	if len(data) != 1 || data[0].(map[string]any)["b64_json"] != base64.StdEncoding.EncodeToString([]byte("watercolor")) {
		t.Fatalf("response = %v", out)
	}
	if out["upstream_model"] != "google/gemini-2.5-flash-image" || resp.Header.Get("X-Nabu-Provider") != "openrouter" {
		t.Fatalf("served by %v / %s", out["upstream_model"], resp.Header.Get("X-Nabu-Provider"))
	}
	content, _ := sent["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if sent["model"] != "google/gemini-2.5-flash-image" || len(content) != 2 ||
		content[1].(map[string]any)["image_url"].(map[string]any)["url"] != "https://example.com/a.png" {
		t.Fatalf("vendor was sent %v", sent)
	}
	if cost, _ := strconv.ParseFloat(resp.Header.Get(costHeader), 64); math.Abs(cost-0.0391) > 1e-9 {
		t.Fatalf("charged %q, want the reported 0.0391", resp.Header.Get(costHeader))
	}
	stat := byModel(tracker)["openrouter/google/gemini-2.5-flash-image"]
	if stat.Requests != 1 || stat.Images != 1 || math.Abs(stat.ProviderCostUSD-0.0391) > 1e-9 || stat.UnpricedRequests != 0 ||
		stat.PromptTokens != 270 || stat.CompletionTokens != 1290 {
		t.Fatalf("metered %+v", stat)
	}

	// A key without the openrouter/* grant is refused before any vendor call.
	sent = nil
	if resp, out := postImagesAs(t, ts, "chat-only", `{"model":"openrouter/google/gemini-2.5-flash-image","prompt":"p"}`); resp.StatusCode != http.StatusForbidden || sent != nil {
		t.Fatalf("chat-only key: %d %v", resp.StatusCode, out)
	}
}

// With no cost in the answer the call is priced the usual way: by the table
// when it knows the model, and booked unpriced when it does not.
func TestOpenRouterImageWithoutAReportedCost(t *testing.T) {
	answer := `{"choices":[{"message":{"images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,aGk="}}]}}],"usage":{"prompt_tokens":5,"completion_tokens":1290,"total_tokens":1295}}`
	vendor := openRouterVendor(t, answer, nil).URL

	ts, tracker := openRouterImageServer(t, vendor, map[string]usage.Price{"gemini-3.1-flash-image": {PerImage: 0.067}})
	resp, out := postImagesAs(t, ts, "gen-key", `{"model":"openrouter/google/gemini-3.1-flash-image","prompt":"p"}`)
	if cost, _ := strconv.ParseFloat(resp.Header.Get(costHeader), 64); resp.StatusCode != http.StatusOK || math.Abs(cost-0.067) > 1e-9 {
		t.Fatalf("table-priced: %d, cost %q, %v", resp.StatusCode, resp.Header.Get(costHeader), out)
	}

	ts, tracker = openRouterImageServer(t, vendor, nil)
	resp, out = postImagesAs(t, ts, "gen-key", `{"model":"openrouter/openai/gpt-5-image","prompt":"p"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unpriced: %d %v", resp.StatusCode, out)
	}
	if stat := byModel(tracker)["openrouter/openai/gpt-5-image"]; stat.Requests != 1 || stat.UnpricedRequests != 1 || stat.CostUSD != 0 {
		t.Fatalf("metered %+v; want one unpriced request", stat)
	}
}
