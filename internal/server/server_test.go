package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nabugate/internal/agent"
	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// fakeUpstream is a single OpenAI-wire provider serving /models,
// /chat/completions and /responses for the server integration tests.
func fakeUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"openai/gpt-5.5"},{"id":"google/gemini-2.5-flash"}]}`))
		case "/chat/completions":
			var b map[string]json.RawMessage
			_ = json.NewDecoder(r.Body).Decode(&b)
			var m string
			_ = json.Unmarshal(b["model"], &m)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi from ` + m + `"},"finish_reason":"stop"}],"usage":{"total_tokens":3}}`))
		case "/responses":
			raw, _ := io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(append([]byte(`{"echo":`), append(raw, '}')...))
		default:
			http.NotFound(w, r)
		}
	}))
}

// newTestServer wires a Server whose only provider is a passthrough-enabled
// OpenAI adapter pointed at the given upstream, with one alias (nabu-fast).
// agents may be nil when the test does not exercise sub-agents.
func newTestServer(t *testing.T, upstreamURL string, enforcer *policy.Enforcer, agents *agent.Registry) *httptest.Server {
	t.Helper()
	adapters := map[string]provider.Adapter{
		"parspack": provider.NewOpenAIAdapter("parspack", upstreamURL, "k", nil),
	}
	models := map[string]config.ModelRoute{
		"nabu-fast": {Primary: config.Target{Provider: "parspack", Model: "openai/gpt-5.5"}},
	}
	r := router.New(adapters, models, nil, nil, nil, map[string][]string{"parspack": nil}, discardLogger())
	srv := New(r, enforcer, usage.New(nil), agents, discardLogger())
	return httptest.NewServer(srv.Handler())
}

// TestModelsIncludesDiscovered verifies /v1/models unions the configured alias
// with the passthrough provider's live-discovered catalogue.
func TestModelsIncludesDiscovered(t *testing.T) {
	up := fakeUpstream(t)
	defer up.Close()
	ts := newTestServer(t, up.URL, policy.New(nil, nil), nil) // auth disabled
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)

	ids := make(map[string]string)
	for _, m := range out.Data {
		ids[m.ID] = m.OwnedBy
	}
	for _, want := range []string{"nabu-fast", "parspack/openai/gpt-5.5", "parspack/google/gemini-2.5-flash"} {
		if _, ok := ids[want]; !ok {
			t.Errorf("model %q missing from /v1/models (got %v)", want, ids)
		}
	}
	if ids["parspack/openai/gpt-5.5"] != "parspack" {
		t.Errorf("owned_by = %q, want parspack", ids["parspack/openai/gpt-5.5"])
	}
}

// TestPassthroughChat verifies a chat request addressed directly as
// "<provider>/<model>" (no alias) routes through and rewrites the upstream model.
func TestPassthroughChat(t *testing.T) {
	up := fakeUpstream(t)
	defer up.Close()
	ts := newTestServer(t, up.URL, policy.New(nil, nil), nil)
	defer ts.Close()

	body := `{"model":"parspack/openai/gpt-5.5","messages":[{"role":"user","content":"hi"}]}`
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Nabu-Provider"); got != "parspack" {
		t.Errorf("X-Nabu-Provider = %q", got)
	}
	var out struct {
		UpstreamModel string `json:"upstream_model"`
		Choices       []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.UpstreamModel != "openai/gpt-5.5" {
		t.Errorf("upstream_model = %q", out.UpstreamModel)
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content != "hi from openai/gpt-5.5" {
		t.Errorf("unexpected content: %+v", out.Choices)
	}
}

// TestResponsesPassthrough verifies /v1/responses proxies to the provider,
// rewriting only "model", and streams the upstream body back.
func TestResponsesPassthrough(t *testing.T) {
	up := fakeUpstream(t)
	defer up.Close()
	ts := newTestServer(t, up.URL, policy.New(nil, nil), nil)
	defer ts.Close()

	body := `{"model":"parspack/openai/gpt-5.5","input":"hi","reasoning":{"effort":"medium"}}`
	resp, err := http.Post(ts.URL+"/v1/responses", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	// Upstream echoed the (model-rewritten) request body back inside {"echo":…}.
	var out struct {
		Echo map[string]json.RawMessage `json:"echo"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("bad proxy body %s: %v", raw, err)
	}
	var gotModel string
	_ = json.Unmarshal(out.Echo["model"], &gotModel)
	if gotModel != "openai/gpt-5.5" {
		t.Errorf("model forwarded = %q, want openai/gpt-5.5", gotModel)
	}
	if _, ok := out.Echo["reasoning"]; !ok {
		t.Error("reasoning param not forwarded through /v1/responses")
	}
}

// TestPolicyFiltersPassthroughModels verifies a provider-wildcard grant both
// reveals the provider's models on /v1/models and gates chat access, while a
// key without the grant sees neither and is forbidden.
func TestPolicyFiltersPassthroughModels(t *testing.T) {
	up := fakeUpstream(t)
	defer up.Close()
	enforcer := policy.New(nil, []policy.KeyConfig{
		{Key: "wide", Project: "p", Allow: []string{"parspack/*"}},
		{Key: "narrow", Project: "q", Allow: []string{"nabu-fast"}},
	})
	ts := newTestServer(t, up.URL, enforcer, nil)
	defer ts.Close()

	listFor := func(key string) map[string]bool {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		got := map[string]bool{}
		for _, m := range out.Data {
			got[m.ID] = true
		}
		return got
	}

	if got := listFor("wide"); !got["parspack/openai/gpt-5.5"] {
		t.Errorf("wide key should see parspack models, got %v", got)
	}
	if got := listFor("narrow"); got["parspack/openai/gpt-5.5"] {
		t.Errorf("narrow key must not see parspack models, got %v", got)
	}

	// narrow key is forbidden from chatting to a parspack passthrough model.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"parspack/openai/gpt-5.5","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer narrow")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("narrow key chat status = %d, want 403", resp.StatusCode)
	}
}
