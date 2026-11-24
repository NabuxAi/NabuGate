package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nabugate/internal/adminstore"
	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// demoUpstream answers every chat and remembers the last body it was sent.
type demoUpstream struct {
	mu   sync.Mutex
	last map[string]json.RawMessage
	fail bool
}

func (u *demoUpstream) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&b)
		u.mu.Lock()
		u.last = b
		fail := u.fail
		u.mu.Unlock()
		if fail {
			http.Error(w, `{"error":{"message":"down"}}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Hello from the demo"},"finish_reason":"stop"}],"usage":{"prompt_tokens":40,"completion_tokens":10,"total_tokens":50}}`))
	}))
}

func newDemoServer(t *testing.T, upstreamURL string) *Server {
	t.Helper()
	t.Setenv("NABU_DEMO_ALIAS", "")
	t.Setenv("NABU_DEMO_DAILY_USD", "")
	adapters := map[string]provider.Adapter{"vendor": provider.NewOpenAIAdapter("vendor", upstreamURL, "k", nil)}
	models := map[string]config.ModelRoute{
		"nabu-fast": {
			Primary:  config.Target{Provider: "vendor", Model: "fast-model"},
			Fallback: []config.Target{{Provider: "vendor", Model: "backup-model"}},
		},
	}
	r := router.New(adapters, models, nil, nil, nil, nil, nil, discardLogger())
	tracker := usage.New(map[string]usage.Price{"vendor/fast-model": {Input: 1, Output: 4}})
	return New(r, policy.New(nil, nil), tracker, nil, discardLogger())
}

func tryDemo(t *testing.T, h http.Handler, ip, body string) (*http.Response, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/public/try", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", ip)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Result(), out
}

func errorCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

// A visitor's prompt is answered by the demo alias, and the answer comes back
// with where it was served, what it cost and how many tries are left. The
// call is metered under its own project.
func TestDemoAnswersThroughTheDemoAlias(t *testing.T) {
	up := &demoUpstream{}
	ts := up.server(t)
	defer ts.Close()
	srv := newDemoServer(t, ts.URL)

	resp, out := tryDemo(t, srv.Handler(), "203.0.113.1", `{"prompt":"What is NabuGate?","lang":"en","model":"gpt-5.5-pro","max_tokens":9000}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %v", resp.StatusCode, out)
	}
	if out["text"] != "Hello from the demo" || out["provider"] != "vendor" || out["model"] != "fast-model" || out["alias"] != "nabu-fast" {
		t.Fatalf("answer = %v", out)
	}
	if chain, _ := out["chain"].([]any); len(chain) != 2 || chain[0] != "fast-model" || chain[1] != "backup-model" {
		t.Fatalf("chain = %v, want the alias's models in order", out["chain"])
	}
	if out["remaining"] != float64(demoTriesPerHour-1) {
		t.Fatalf("remaining = %v", out["remaining"])
	}
	// 40 prompt tokens at $1/M plus 10 completion tokens at $4/M.
	if cost, _ := out["cost_usd"].(float64); cost < 0.00007999 || cost > 0.00008001 {
		t.Fatalf("cost_usd = %v, want 0.00008", out["cost_usd"])
	}

	// The caller chose neither the model nor the answer length.
	up.mu.Lock()
	var model string
	var maxTokens int
	_ = json.Unmarshal(up.last["model"], &model)
	_ = json.Unmarshal(up.last["max_tokens"], &maxTokens)
	up.mu.Unlock()
	if model != "fast-model" || maxTokens != demoMaxTokens {
		t.Fatalf("upstream got model %q max_tokens %d, want fast-model and %d", model, maxTokens, demoMaxTokens)
	}

	byProject, _ := srv.usage.Snapshot()
	if byProject[demoProject].Requests != 1 {
		t.Fatalf("demo usage not metered under %q: %v", demoProject, byProject)
	}
}

// Five tries per visitor per hour; another visitor is unaffected.
func TestDemoLimitsTriesPerVisitor(t *testing.T) {
	up := &demoUpstream{}
	ts := up.server(t)
	defer ts.Close()
	h := newDemoServer(t, ts.URL).Handler()

	for i := 0; i < demoTriesPerHour; i++ {
		if resp, out := tryDemo(t, h, "203.0.113.2", `{"prompt":"hi"}`); resp.StatusCode != http.StatusOK {
			t.Fatalf("try %d = %d %v", i+1, resp.StatusCode, out)
		}
	}
	resp, out := tryDemo(t, h, "203.0.113.2", `{"prompt":"hi"}`)
	if resp.StatusCode != http.StatusTooManyRequests || errorCode(out) != "demo_rate_limited" {
		t.Fatalf("sixth try = %d %v, want 429 demo_rate_limited", resp.StatusCode, out)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("rate-limited try has no Retry-After")
	}
	if resp, _ := tryDemo(t, h, "203.0.113.3", `{"prompt":"hi"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("another visitor = %d, want 200", resp.StatusCode)
	}
}

// Once the day's spend reaches the cap, every try is refused until the day
// turns.
func TestDemoStopsAtTheDailyCap(t *testing.T) {
	g := newDemoGate()
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	if code, _, _ := g.admit("a", now, 2); code != "" {
		t.Fatalf("first try refused: %s", code)
	}
	g.done(now, 2.5)
	if code, _, _ := g.admit("b", now, 2); code != "demo_budget" {
		t.Fatalf("over the cap: code %q, want demo_budget", code)
	}
	if code, _, _ := g.admit("b", now.Add(15*time.Hour), 2); code != "" {
		t.Fatalf("next UTC day: code %q, want admitted", code)
	}
}

// A failed answer does not cost the visitor a try.
func TestDemoFailureIsRefunded(t *testing.T) {
	up := &demoUpstream{fail: true}
	ts := up.server(t)
	defer ts.Close()
	srv := newDemoServer(t, ts.URL)

	resp, out := tryDemo(t, srv.Handler(), "203.0.113.4", `{"prompt":"hi"}`)
	if resp.StatusCode != http.StatusBadGateway || errorCode(out) != "upstream_failed" {
		t.Fatalf("failed try = %d %v, want 502 upstream_failed", resp.StatusCode, out)
	}
	if _, remaining, _ := srv.demo.admit("203.0.113.4", time.Now(), 2); remaining != demoTriesPerHour-1 {
		t.Fatalf("after a failure the visitor has %d tries left, want %d", remaining, demoTriesPerHour-1)
	}
	if srv.demo.inFlight != 1 {
		t.Fatalf("in-flight = %d after one finished failure and one admit, want 1", srv.demo.inFlight)
	}
}

func TestDemoRefusesBadInputAndCanBeTurnedOff(t *testing.T) {
	up := &demoUpstream{}
	ts := up.server(t)
	defer ts.Close()
	srv := newDemoServer(t, ts.URL)
	h := srv.Handler()

	for _, body := range []string{`{"prompt":"   "}`, `{"prompt":"` + strings.Repeat("ب", demoMaxPromptRune+1) + `"}`, `not json`} {
		if resp, out := tryDemo(t, h, "203.0.113.5", body); resp.StatusCode != http.StatusBadRequest || errorCode(out) != "bad_prompt" {
			t.Fatalf("body %.30q = %d %v, want 400 bad_prompt", body, resp.StatusCode, out)
		}
	}
	// Exactly the limit is fine, counted in characters rather than bytes.
	if resp, out := tryDemo(t, h, "203.0.113.5", `{"prompt":"`+strings.Repeat("ب", demoMaxPromptRune)+`"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("400-character Persian prompt = %d %v", resp.StatusCode, out)
	}

	t.Setenv("NABU_DEMO_DAILY_USD", "0")
	if resp, out := tryDemo(t, h, "203.0.113.6", `{"prompt":"hi"}`); resp.StatusCode != http.StatusServiceUnavailable || errorCode(out) != "demo_disabled" {
		t.Fatalf("cap 0 = %d %v, want 503 demo_disabled", resp.StatusCode, out)
	}
	t.Setenv("NABU_DEMO_DAILY_USD", "")
	t.Setenv("NABU_DEMO_ALIAS", "no-such-alias")
	if resp, out := tryDemo(t, h, "203.0.113.6", `{"prompt":"hi"}`); resp.StatusCode != http.StatusServiceUnavailable || errorCode(out) != "demo_disabled" {
		t.Fatalf("unknown alias = %d %v, want 503 demo_disabled", resp.StatusCode, out)
	}
}

func beacon(t *testing.T, s *Server, ua, ip, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/public/engagement", strings.NewReader(body))
	req.Header.Set("User-Agent", ua)
	req.Header.Set("X-Forwarded-For", ip)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("beacon answered %d, want 204", rec.Code)
	}
}

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0 Safari/537.36"

// A view is counted once, clamped and filtered; bots and junk are not counted
// at all, and every answer is a 204.
func TestEngagementBeaconCountsRealViews(t *testing.T) {
	store, err := adminstore.Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := newDemoServer(t, "http://127.0.0.1:1")
	srv.SetAdminStore(store)

	beacon(t, srv, browserUA, "198.51.100.1", `{"page":"home","lang":"en","seconds":95.4,"scroll":80,"sections":["try","pricing","pricing","evil-key"],"tried":true}`)
	beacon(t, srv, browserUA, "198.51.100.2", `{"page":"home","lang":"xx","seconds":99999,"scroll":140,"cta":true}`)
	beacon(t, srv, "Googlebot/2.1", "198.51.100.3", `{"page":"home","lang":"en","seconds":20}`)
	beacon(t, srv, browserUA, "198.51.100.4", `{"page":"admin","seconds":20}`)
	beacon(t, srv, browserUA, "198.51.100.5", `garbage`)

	report := store.EngagementReport(time.Now(), 7)
	if len(report) != 1 || report[0].Page != "home" {
		t.Fatalf("report = %+v, want one home day", report)
	}
	d := report[0]
	if d.Views != 2 {
		t.Fatalf("views = %d, want 2 (bot, unknown page and junk dropped)", d.Views)
	}
	if d.Seconds != 95+engagementMaxSecond {
		t.Fatalf("seconds = %d, want 95 + the clamp", d.Seconds)
	}
	if d.Time != [5]int64{0, 0, 0, 1, 1} {
		t.Fatalf("time buckets = %v", d.Time)
	}
	if d.Scroll != [4]int64{2, 2, 2, 1} {
		t.Fatalf("scroll buckets = %v", d.Scroll)
	}
	if d.Sections["pricing"] != 1 || d.Sections["try"] != 1 || len(d.Sections) != 2 {
		t.Fatalf("sections = %v, want try and pricing once each", d.Sections)
	}
	if d.Langs["en"] != 1 || d.Langs["fa"] != 1 {
		t.Fatalf("langs = %v, want an unknown language counted as fa", d.Langs)
	}
	if d.Tries != 1 || d.CTA != 1 {
		t.Fatalf("tries %d cta %d, want 1 and 1", d.Tries, d.CTA)
	}

	// One address cannot inflate the numbers.
	for i := 0; i < beaconsPerHour+10; i++ {
		beacon(t, srv, browserUA, "198.51.100.9", `{"page":"docs","seconds":5}`)
	}
	for _, day := range store.EngagementReport(time.Now(), 7) {
		if day.Page == "docs" && day.Views != beaconsPerHour {
			t.Fatalf("docs views from one address = %d, want capped at %d", day.Views, beaconsPerHour)
		}
	}
}

// The counters survive a restart, and old days age out.
func TestEngagementPersistsAndPrunes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin.json")
	store, err := adminstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	store.RecordEngagement(now.AddDate(0, 0, -120), adminstore.EngagementView{Page: "home", Lang: "fa", Seconds: 5})
	store.RecordEngagement(now, adminstore.EngagementView{Page: "home", Lang: "fa", Seconds: 40, Scroll: 50})
	if err := store.Persist(); err != nil {
		t.Fatal(err)
	}
	again, err := adminstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	report := again.EngagementReport(now, 90)
	if len(report) != 1 || report[0].Day != "2026-11-24" || report[0].Views != 1 || report[0].Time[2] != 1 {
		t.Fatalf("after reopen: %+v, want only today's one view", report)
	}
}

// The admin report says what the demo has spent today against its cap.
func TestEngagementReportShowsDemoSpend(t *testing.T) {
	store, err := adminstore.Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := newDemoServer(t, "http://127.0.0.1:1")
	srv.SetAdminStore(store)
	now := time.Now()
	srv.demo.admit("x", now, 2)
	srv.demo.done(now, 0.25)

	rec := httptest.NewRecorder()
	srv.engagementReport(rec, httptest.NewRequest(http.MethodGet, "/api/engagement?days=7", nil))
	var out struct {
		Days []any `json:"days"`
		Demo struct {
			Spent float64 `json:"spent_today_usd"`
			Cap   float64 `json:"daily_cap_usd"`
		} `json:"demo"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Demo.Spent != 0.25 || out.Demo.Cap != 2 || out.Days == nil {
		t.Fatalf("report = %s", rec.Body.String())
	}
}
