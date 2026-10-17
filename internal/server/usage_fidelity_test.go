package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nabugate/internal/adminstore"
	"nabugate/internal/agent"
	"nabugate/internal/config"
	"nabugate/internal/flow"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// ───────────────────────── usage fidelity, end to end ───────────────────────
//
// The data layer (internal/usage) can carry both costs and the honesty
// counters; these tests walk real requests through the gateway and read them
// back off /v1/usage, which is where the operator actually looks.

// fidelityServer builds a gateway whose nabu-fast alias serves
// parspack/openai/gpt-5.5 through the given upstream (for chat, streams and
// embeddings alike), priced at a dollar per million prompt tokens — so
// provider_cost_usd and cost_usd are both observable, and any model outside
// the table is observably unpriced.
func fidelityServer(t *testing.T, upstreamURL string, agents *agent.Registry, flows *flow.Registry) (*httptest.Server, *usage.Tracker) {
	t.Helper()
	adapters := map[string]provider.Adapter{
		"parspack": provider.NewOpenAIAdapter("parspack", upstreamURL, "k", nil),
	}
	alias := config.ModelRoute{Primary: config.Target{Provider: "parspack", Model: "openai/gpt-5.5"}}
	r := router.New(adapters,
		map[string]config.ModelRoute{"nabu-fast": alias},
		nil, nil,
		map[string]config.ModelRoute{"nabu-fast": alias},
		nil,
		map[string][]string{"parspack": nil}, discardLogger())
	tracker := usage.New(map[string]usage.Price{"parspack/openai/gpt-5.5": {Input: 1, Output: 1}})
	srv := New(r, policy.New(nil, nil), tracker, agents, discardLogger()).WithFlows(flows)
	return httptest.NewServer(srv.Handler()), tracker
}

// usageSnapshot GETs /v1/usage (auth is off on these servers) and decodes it.
func usageSnapshot(t *testing.T, ts *httptest.Server) (byProject, byModel map[string]usage.Stat) {
	t.Helper()
	resp, err := http.Get(ts.URL + "/v1/usage")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		ByProject map[string]usage.Stat `json:"by_project"`
		ByModel   map[string]usage.Stat `json:"by_model"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode /v1/usage: %v", err)
	}
	return out.ByProject, out.ByModel
}

const modelKey = "parspack/openai/gpt-5.5"

// A chat on a model with no price in the config used to be indistinguishable
// from a free month. Now the tokens land (estimated, because the upstream
// reports nothing at all), the cost stays the zero it was, and the row
// carries unpriced_requests=1 — the operator can see which model to go and
// price.
func TestUnpricedChatIsVisibleAsUnpriced(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"an answer without any usage object"},"finish_reason":"stop"}]}`))
	}))
	defer up.Close()
	ts, _ := fidelityServer(t, up.URL, nil, nil) // priced for gpt-5.5 — so use the bare passthrough name
	defer ts.Close()

	body := `{"model":"parspack/other-vendor/model-nobody-priced","messages":[{"role":"user","content":"hi"}]}`
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("chat status = %d", resp.StatusCode)
	}

	_, byModel := usageSnapshot(t, ts)
	row := byModel["parspack/other-vendor/model-nobody-priced"]
	if row.PromptTokens <= 0 {
		t.Fatalf("prompt_tokens = %d, want the real (estimated) count", row.PromptTokens)
	}
	if row.CostUSD != 0 {
		t.Errorf("cost_usd = %v, want 0 — the charge contract for an unpriced model is unchanged", row.CostUSD)
	}
	if row.UnpricedRequests != 1 {
		t.Errorf("unpriced_requests = %d, want 1", row.UnpricedRequests)
	}
	if row.EstimatedRequests != 1 {
		t.Errorf("estimated_requests = %d, want 1 — the upstream reported no usage at all", row.EstimatedRequests)
	}
}

func sseChatStream(t *testing.T, ts *httptest.Server, body string) string {
	t.Helper()
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, raw)
	}
	return string(raw)
}

// include_usage is OpenAI's own opt-in: a caller that asked for it gets a
// final, empty-choices usage chunk carrying the gateway's charge; a caller
// that did not ask gets not one extra byte.
func TestStreamUsageChunkHonoursIncludeUsage(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":5,\"total_tokens\":12}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer up.Close()
	ts, _ := fidelityServer(t, up.URL, nil, nil)
	defer ts.Close()

	with := sseChatStream(t, ts, `{"model":"nabu-fast","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hi"}]}`)
	// The usage chunk: empty choices, the vendor's counts, and the gateway's
	// charge riding alongside. Keys are marshalled alphabetically, so assert
	// on the fields rather than one exact substring.
	usageChunk := ""
	for _, line := range strings.Split(with, "\n") {
		if strings.Contains(line, `"usage"`) {
			usageChunk = line
		}
	}
	if usageChunk == "" {
		t.Fatalf("include_usage stream carries no usage chunk:\n%s", with)
	}
	for _, want := range []string{`"choices":[]`, `"prompt_tokens":7`, `"completion_tokens":5`, `"total_tokens":12`, `"cost_usd":`} {
		if !strings.Contains(usageChunk, want) {
			t.Errorf("usage chunk missing %s: %s", want, usageChunk)
		}
	}
	if !strings.Contains(with, "data: [DONE]") {
		t.Error("the stream must still end with [DONE]")
	}

	without := sseChatStream(t, ts, `{"model":"nabu-fast","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if strings.Contains(without, `"usage"`) {
		t.Errorf("a caller that never asked for include_usage got one anyway:\n%s", without)
	}

	// Either way the call is metered exactly once.
	_, byModel := usageSnapshot(t, ts)
	if row := byModel[modelKey]; row.Requests != 2 || row.PromptTokens != 14 {
		t.Errorf("by_model row = %+v, want two requests at 7 prompt tokens each", row)
	}
}

// A stream that the upstream cut mid-flight after having already metered work
// keeps that work visible: failed_requests=1 with the real tokens and the
// vendor's cost on the books, the caller charged nothing, and no served
// request counted. The fixture cuts the connection mid-chunk (an aborted
// handler), which the HTTP client reads as an unexpected EOF — the shape the
// critic's review asked for.
func TestMidCutStreamKeepsItsRealUsageAsFailed(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello \"}}]}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"world\"}}],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":5,\"total_tokens\":12}}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Half a data line, then abort: the chunked body never finishes, so
		// the gateway's read fails rather than closing cleanly.
		fmt.Fprint(w, "data: {\"cho")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		panic(http.ErrAbortHandler)
	}))
	defer up.Close()
	ts, _ := fidelityServer(t, up.URL, nil, nil)
	defer ts.Close()

	raw := sseChatStream(t, ts, `{"model":"nabu-fast","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if !strings.Contains(raw, `"finish_reason":"error"`) {
		t.Errorf("the caller should still see the stream end with an error finish:\n%s", raw)
	}

	_, byModel := usageSnapshot(t, ts)
	row := byModel[modelKey]
	if row.Requests != 0 {
		t.Errorf("requests = %d, want 0 — the request never completed", row.Requests)
	}
	if row.FailedRequests != 1 {
		t.Errorf("failed_requests = %d, want 1", row.FailedRequests)
	}
	if row.PromptTokens != 7 || row.CompletionTokens != 5 {
		t.Errorf("tokens = %d/%d, want the upstream's real 7/5", row.PromptTokens, row.CompletionTokens)
	}
	if row.CostUSD != 0 {
		t.Errorf("cost_usd = %v, want 0 — the caller is not billed for a broken stream", row.CostUSD)
	}
	if row.ProviderCostUSD <= 0 {
		t.Errorf("provider_cost_usd = %v, want the vendor's list price for the tokens actually spent", row.ProviderCostUSD)
	}
}

// The embeddings response shows the same number the metering booked: when the
// upstream reported no usage, the caller reads the estimate and is told it is
// one, instead of a vendor-zero that disagrees with the books.
func TestEmbeddingsResponseCarriesTheMeteredTruth(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]},{"embedding":[0.3,0.4]}]}`))
	}))
	defer up.Close()
	ts, _ := fidelityServer(t, up.URL, nil, nil)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/embeddings", "application/json",
		strings.NewReader(`{"model":"nabu-fast","input":"how long is this piece of text, then"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Usage struct {
			PromptTokens int   `json:"prompt_tokens"`
			TotalTokens  int   `json:"total_tokens"`
			CostUSD      any   `json:"cost_usd"`
			Estimated    *bool `json:"estimated"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Usage.PromptTokens <= 0 {
		t.Fatalf("usage.prompt_tokens = %d, want the estimate that was metered", out.Usage.PromptTokens)
	}
	if out.Usage.Estimated == nil || !*out.Usage.Estimated {
		t.Error("the response must say the counts are an estimate")
	}
	_, byModel := usageSnapshot(t, ts)
	if row := byModel[modelKey]; row.PromptTokens != int64(out.Usage.PromptTokens) || row.EstimatedRequests != 1 {
		t.Errorf("books = %+v, must agree with the response's %d", row, out.Usage.PromptTokens)
	}
}

// ───────────────────────────── /v1/responses ────────────────────────────────

func responsesUpstream(t *testing.T, body, contentType string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// A JSON Responses answer with a usage object is metered from it, at the
// vendor's own numbers, with the caller charged as on any other route.
func TestResponsesJSONUsageIsMetered(t *testing.T) {
	up := responsesUpstream(t,
		`{"id":"resp_1","status":"completed","usage":{"input_tokens":100,"output_tokens":30,"total_tokens":130}}`,
		"application/json", http.StatusOK)
	defer up.Close()
	ts, _ := fidelityServer(t, up.URL, nil, nil)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/responses", "application/json",
		strings.NewReader(`{"model":"nabu-fast","input":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	_, byModel := usageSnapshot(t, ts)
	row := byModel[modelKey]
	if row.Requests != 1 || row.PromptTokens != 100 || row.CompletionTokens != 30 {
		t.Fatalf("by_model row = %+v, want one request at the vendor's 100/30", row)
	}
	if row.CostUSD <= 0 || row.ProviderCostUSD <= 0 {
		t.Errorf("costs = %v charged / %v vendor, both want > 0", row.CostUSD, row.ProviderCostUSD)
	}
	if row.EstimatedRequests != 0 {
		t.Errorf("estimated_requests = %d, want 0 — the vendor reported real usage", row.EstimatedRequests)
	}
}

// The SSE flavor meters from the response.completed event, and the proxy
// stays byte-identical: metering reads a copy, never the caller's bytes.
func TestResponsesSSEUsageIsMeteredAndBodyUntouched(t *testing.T) {
	upstreamBody := "event: response.completed\n" +
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8}}}` + "\n\n"
	up := responsesUpstream(t, upstreamBody, "text/event-stream", http.StatusOK)
	defer up.Close()
	ts, _ := fidelityServer(t, up.URL, nil, nil)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/responses", "application/json",
		strings.NewReader(`{"model":"nabu-fast","input":"hi","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != upstreamBody {
		t.Fatalf("proxied body not byte-identical:\ngot  %q\nwant %q", raw, upstreamBody)
	}

	_, byModel := usageSnapshot(t, ts)
	if row := byModel[modelKey]; row.Requests != 1 || row.PromptTokens != 5 || row.CompletionTokens != 3 {
		t.Fatalf("by_model row = %+v, want one request at 5/3", row)
	}
}

// A Responses answer with no usage anywhere is still one request on the
// books — estimated from the body sizes and flagged — never a silent zero.
func TestResponsesWithoutUsageIsEstimated(t *testing.T) {
	up := responsesUpstream(t, `{"id":"resp_2","status":"completed","output":"hi"}`, "application/json", http.StatusOK)
	defer up.Close()
	ts, _ := fidelityServer(t, up.URL, nil, nil)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/responses", "application/json",
		strings.NewReader(`{"model":"nabu-fast","input":"a moderately long input to be estimated","instructions":"be brief"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	_, byModel := usageSnapshot(t, ts)
	row := byModel[modelKey]
	if row.Requests != 1 {
		t.Errorf("requests = %d, want 1", row.Requests)
	}
	if row.PromptTokens <= 0 || row.CompletionTokens <= 0 {
		t.Fatalf("tokens = %d/%d, want the size-based estimates", row.PromptTokens, row.CompletionTokens)
	}
	if row.EstimatedRequests != 1 {
		t.Errorf("estimated_requests = %d, want 1", row.EstimatedRequests)
	}
}

// An upstream error status passes through untouched and meters nothing: an
// error body carries no work worth billing, on the wire or on the books.
func TestResponsesErrorStatusIsNotMetered(t *testing.T) {
	up := responsesUpstream(t, `{"error":{"message":"nope"}}`, "application/json", http.StatusBadRequest)
	defer up.Close()
	ts, _ := fidelityServer(t, up.URL, nil, nil)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/responses", "application/json",
		strings.NewReader(`{"model":"nabu-fast","input":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want the upstream's 400 passed through", resp.StatusCode)
	}

	_, byModel := usageSnapshot(t, ts)
	if row, ok := byModel[modelKey]; ok && row.Requests != 0 {
		t.Fatalf("by_model row = %+v, want nothing metered", row)
	}
}

// ───────────────────── tool loop and flow metering ──────────────────────────

// A tool loop whose next provider step fails keeps the steps it already paid
// for: they land as failed work with their real tokens and vendor cost, the
// caller is charged nothing, and the failure is a row instead of a hole.
func TestToolLoopFailureKeepsTheStepsItPaidFor(t *testing.T) {
	toolSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":42,"status":"shipped"}`)
	}))
	defer toolSrv.Close()

	// The provider answers the first turn with a tool call, then — once the
	// tool result is in the conversation — errors. That failing step is the
	// one whose failure used to swallow step one's usage entirely.
	failUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&b)
		var msgs []struct {
			Role string `json:"role"`
		}
		_ = json.Unmarshal(b["messages"], &msgs)
		for _, m := range msgs {
			if m.Role == "tool" {
				http.Error(w, `{"error":{"message":"upstream is having a bad minute"}}`, http.StatusInternalServerError)
				return
			}
		}
		fmt.Fprint(w, toolCallReply("call_1", "track_order", `{"order_id":"42"}`))
	}))
	defer failUp.Close()

	ts := newToolTestServer(t, failUp.URL, shopAgent(t, toolSrv.URL, 0))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"shop-agent","messages":[{"role":"user","content":"سفارش ۴۲ کجاست؟"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}

	_, byModel := usageSnapshot(t, ts)
	row := byModel[modelKey]
	if row.FailedRequests != 1 {
		t.Errorf("failed_requests = %d, want 1", row.FailedRequests)
	}
	if row.PromptTokens != 10 || row.CompletionTokens != 5 {
		t.Errorf("tokens = %d/%d, want the first step's real 10/5", row.PromptTokens, row.CompletionTokens)
	}
	if row.Requests != 0 || row.CostUSD != 0 {
		t.Errorf("requests/cost = %d/%v, want 0/0 — nothing was served and nobody is billed", row.Requests, row.CostUSD)
	}
}

// A nested flow's inner steps are billed under the provider and model that
// actually served them — a second-level flow's spend used to vanish because
// its parent step carried no provider of its own.
func TestNestedFlowStepsAreMeteredAtTheirOwnProvider(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)

	agents := agent.NewRegistry()
	for _, name := range []string{"writer", "reviewer"} {
		_ = agents.Add(agent.Agent{Name: name, Model: "nabu-fast", System: "you are the " + name})
	}
	flows := flow.NewRegistry()
	if err := flows.Add(flow.Flow{
		Name:        "inner",
		Description: "the second-level flow",
		Steps:       []flow.Step{{Agent: "writer"}, {Agent: "reviewer"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := flows.Add(flow.Flow{
		Name:        "outer",
		Description: "calls the inner flow",
		Steps:       []flow.Step{{Agent: "inner"}},
	}); err != nil {
		t.Fatal(err)
	}

	ts, _ := fidelityServer(t, up.URL, agents, flows)
	defer ts.Close()

	resp, out := postFlow(t, ts, `{"model":"outer","messages":[{"role":"user","content":"brief"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("chat status = %d: %v", resp.StatusCode, out)
	}

	_, byModel := usageSnapshot(t, ts)
	row := byModel[modelKey]
	// Two inner steps, each metered at the provider/model that served it.
	if row.Requests != 2 {
		t.Errorf("requests = %d, want 2 — one per inner step", row.Requests)
	}
	if row.PromptTokens != 2 {
		t.Errorf("prompt_tokens = %d, want 2 (1 per echo step)", row.PromptTokens)
	}
	if row.CostUSD <= 0 {
		t.Errorf("cost_usd = %v, want both steps charged", row.CostUSD)
	}
}

// ───────────────────────────── /api/me/usage ────────────────────────────────

// The panel's per-user rollup now sums the vendor's side of the ledger next
// to the charged one; the keys the panel already reads are unchanged.
func TestAccountUsageSumsBothCosts(t *testing.T) {
	store, err := adminstore.Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The rollup is scoped by ownership: the token names the project, the
	// owner names the token, exactly as the panel resolves it.
	if _, _, err := store.NewToken("mine", []string{"*"}, 0, nil, "me@example.com", nil); err != nil {
		t.Fatalf("mint key: %v", err)
	}
	store.RecordUsage("mine", "openai", "gpt", adminstore.UsageEntry{Requests: 1, PromptTokens: 10, CompletionTokens: 10, CostUSD: 0.01, ProviderCostUSD: 0.004})
	store.RecordUsage("mine", "anthropic", "claude", adminstore.UsageEntry{Requests: 1, PromptTokens: 20, CompletionTokens: 5, CostUSD: 0.02, ProviderCostUSD: 0.011})

	s := &Server{admin: store}
	r := httptest.NewRequest(http.MethodGet, "/api/me/usage", nil)
	r = r.WithContext(context.WithValue(r.Context(), consoleEmailCtxKey{}, "me@example.com"))
	w := httptest.NewRecorder()
	s.accountUsage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["cost_usd"] != 0.03 {
		t.Errorf("cost_usd = %v, want 0.03", got["cost_usd"])
	}
	if got["provider_cost_usd"] != 0.015 {
		t.Errorf("provider_cost_usd = %v, want 0.015", got["provider_cost_usd"])
	}
	// The shape the panel already reads stays put.
	for _, key := range []string{"projects", "requests", "prompt_tokens", "completion_tokens", "total_tokens", "denied", "cost_usd"} {
		if _, ok := got[key]; !ok {
			t.Errorf("existing key %q missing from /api/me/usage", key)
		}
	}
}

// ─────────────────────────── console flow testing ───────────────────────────

// The console's flow test runs on the gateway's own provider keys, so what it
// spends lands under its own project — the operator sees what testing costs
// and no customer's usage moves.
func TestFlowTestSpendsAreBookedUnderFlowTest(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)
	ts, _, session := consoleFlowServer(t, up.URL)

	resp, out := consoleDo(t, ts, session, http.MethodPost, "/api/flows",
		`{"name":"mine","steps":[{"agent":"writer"},{"agent":"reviewer"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save status = %d: %v", resp.StatusCode, out)
	}

	testResp, testOut := consoleDo(t, ts, session, http.MethodPost, "/api/flows/mine/test", `{"message":"brief"}`)
	if testResp.StatusCode != http.StatusOK {
		t.Fatalf("test status = %d: %v", testResp.StatusCode, testOut)
	}

	_, byModel := usageSnapshot(t, ts)
	row := byModel[modelKey]
	if row.Requests != 2 {
		t.Errorf("requests = %d, want 2 — one per step of the tested flow", row.Requests)
	}
	if row.CostUSD <= 0 {
		t.Errorf("cost_usd = %v, want the steps charged", row.CostUSD)
	}
	byProj, _ := usageSnapshot(t, ts)
	if _, ok := byProj["(flow-test)"]; !ok {
		t.Errorf("the test's spend landed nowhere: by_project = %v", byProj)
	}
}

// The metering capture is capped at 1MiB — but the cap lives in the capture
// writer, not on the proxy: a response longer than the cap must still reach
// the caller whole. (An earlier shape put the limit on the upstream read and
// truncated every big proxied body at exactly 1MiB.) A body whose usage sits
// past the captured prefix then meters by size — the documented fallback —
// rather than by the vendor's numbers.
func TestResponsesLongerThanTheCaptureStillProxyWhole(t *testing.T) {
	filler := strings.Repeat("x", 1<<20) // the usage object lands past the capture cap
	upstreamBody := `{"id":"resp_big","filler":"` + filler + `","usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8}}`
	up := responsesUpstream(t, upstreamBody, "application/json", http.StatusOK)
	defer up.Close()
	ts, _ := fidelityServer(t, up.URL, nil, nil)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/responses", "application/json",
		strings.NewReader(`{"model":"nabu-fast","input":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != len(upstreamBody) || string(raw) != upstreamBody {
		t.Fatalf("proxied body truncated: got %d bytes, want %d", len(raw), len(upstreamBody))
	}

	_, byModel := usageSnapshot(t, ts)
	row := byModel[modelKey]
	if row.Requests != 1 {
		t.Fatalf("requests = %d, want 1", row.Requests)
	}
	if row.EstimatedRequests != 1 || row.PromptTokens <= 0 {
		t.Fatalf("row = %+v, want one size-based estimate with positive tokens", row)
	}
}

// Some OpenAI-compatible servers report the prompt/completion split but omit
// the total. Real counts are never replaced by the byte-based guess — the
// missing total is completed from the split.
func TestResponsesSplitWithoutTotalIsKept(t *testing.T) {
	up := responsesUpstream(t,
		`{"id":"resp_3","status":"completed","usage":{"input_tokens":100,"output_tokens":30}}`,
		"application/json", http.StatusOK)
	defer up.Close()
	ts, _ := fidelityServer(t, up.URL, nil, nil)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/responses", "application/json",
		strings.NewReader(`{"model":"nabu-fast","input":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	_, byModel := usageSnapshot(t, ts)
	row := byModel[modelKey]
	if row.PromptTokens != 100 || row.CompletionTokens != 30 {
		t.Fatalf("by_model row = %+v, want the vendor's own 100/30 kept", row)
	}
	if row.EstimatedRequests != 0 {
		t.Errorf("estimated_requests = %d, want 0 — the vendor reported real usage", row.EstimatedRequests)
	}
}

// A tool loop whose every step the adapter had to guess books its tokens as
// estimated: the flag rides the outcome's sum, not just the single steps.
func TestToolLoopWithoutUsageBooksEstimated(t *testing.T) {
	var toolServed int32
	toolSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":42,"status":"shipped"}`)
	}))
	defer toolSrv.Close()

	// Tool call on turn one, prose on turn two, and no usage object anywhere —
	// every step's counts are the adapter's estimate.
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&b)
		var msgs []struct {
			Role string `json:"role"`
		}
		_ = json.Unmarshal(b["messages"], &msgs)
		for _, m := range msgs {
			if m.Role == "tool" {
				fmt.Fprint(w, `{"choices":[{"message":{"content":"سفارش شما ارسال شد."},"finish_reason":"stop"}]}`)
				return
			}
		}
		fmt.Fprint(w, toolCallReply("call_1", "track_order", `{"order_id":"42"}`))
	}))
	defer up.Close()
	_ = toolServed

	ts := newToolTestServer(t, up.URL, shopAgent(t, toolSrv.URL, 0))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"shop-agent","messages":[{"role":"user","content":"سفارش ۴۲ کجاست؟"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	_, byModel := usageSnapshot(t, ts)
	row := byModel[modelKey]
	if row.Requests != 1 || row.PromptTokens <= 0 {
		t.Fatalf("by_model row = %+v, want one served request with positive tokens", row)
	}
	if row.EstimatedRequests != 1 {
		t.Errorf("estimated_requests = %d, want 1 — every step was a guess", row.EstimatedRequests)
	}
}

// A model served on the caller's own vendor key is charged nothing by the
// gateway and cost it nothing either — so its missing price is not the
// gateway's unpriced hole, and it does not count in unpriced_requests. This
// pins that suppression: an operator chasing unpriced models is looking for
// traffic the gateway paid for.
func TestUnpricedModelOnTheCallersOwnKeyIsNotTheGatewaysHole(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"an answer without any usage object"},"finish_reason":"stop"}]}`))
	}))
	defer up.Close()

	// The caller-adapter wiring a real deployment gets from main.go; without
	// it a caller key is refused rather than used to serve.
	r := router.New(
		map[string]provider.Adapter{"parspack": provider.NewOpenAIAdapter("parspack", up.URL, "k", nil)},
		map[string]config.ModelRoute{"nabu-fast": {Primary: config.Target{Provider: "parspack", Model: "openai/gpt-5.5"}}},
		nil, nil, nil, nil,
		map[string][]string{"parspack": nil}, discardLogger())
	r.SetCallerAdapter(func(providerName, key string) (provider.Adapter, bool) {
		if providerName == "parspack" {
			return provider.NewOpenAIAdapter("parspack", up.URL, key, nil), true
		}
		return nil, false
	})
	srv := New(r, policy.New(nil, nil), usage.New(map[string]usage.Price{"parspack/openai/gpt-5.5": {Input: 1, Output: 1}}), nil, discardLogger())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := `{"model":"parspack/other-vendor/model-nobody-priced","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nabu-Key-Parspack", "the-callers-own-key") // own-first: this serves

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("chat status = %d", resp.StatusCode)
	}

	_, byModel := usageSnapshot(t, ts)
	row := byModel["parspack/other-vendor/model-nobody-priced"]
	if row.Requests != 1 || row.PromptTokens <= 0 {
		t.Fatalf("by_model row = %+v, want the call recorded with its (estimated) tokens", row)
	}
	if row.CostUSD != 0 || row.ProviderCostUSD != 0 {
		t.Errorf("costs = %v charged / %v vendor, want 0/0 — the caller's vendor billed them", row.CostUSD, row.ProviderCostUSD)
	}
	if row.UnpricedRequests != 0 {
		t.Errorf("unpriced_requests = %d, want 0 — the gateway never paid for this call", row.UnpricedRequests)
	}
}

// The caller hanging up mid-stream is not the vendor failing: the tokens
// delivered until then were made for the caller, and are charged.
func TestACallerWhoHangsUpMidStreamPaysForWhatWasDelivered(t *testing.T) {
	release := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\""+strings.Repeat("word ", 400)+"\"}}]}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer up.Close()
	defer close(release)
	ts, _ := fidelityServer(t, up.URL, nil, nil)
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"nabu-fast","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 512)
	if _, err := resp.Body.Read(buf); err != nil {
		t.Fatal(err)
	}
	cancel()
	resp.Body.Close()

	var row usage.Stat
	for i := 0; i < 50; i++ {
		_, byModel := usageSnapshot(t, ts)
		if row = byModel[modelKey]; row.Requests > 0 || row.FailedRequests > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if row.Requests != 1 || row.CostUSD <= 0 {
		t.Fatalf("a caller who hung up was not charged: %+v", row)
	}
}
