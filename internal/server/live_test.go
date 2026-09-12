package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// liveUpstream answers POST /live/sessions the way the vendor does and records
// the body it received.
func liveUpstream(t *testing.T) (*httptest.Server, *[]byte) {
	t.Helper()
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/live/sessions" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		got, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"session":{"id":"live_123"},"transport":{"type":"webrtc","sdp":"v=0 answer"}}`))
	}))
	return srv, &got
}

type liveServerOpts struct {
	statePath  string
	problems   map[string]string
	enforcer   *policy.Enforcer
	callerKeys bool
}

func newLiveServer(t *testing.T, upstreamURL string, o liveServerOpts) (*httptest.Server, *usage.Tracker) {
	t.Helper()
	adapters := map[string]provider.Adapter{
		"openai": provider.NewOpenAIAdapter("openai", upstreamURL, "k", nil),
	}
	r := router.New(adapters, nil, nil, nil, nil, nil, nil, discardLogger())
	r.SetLive(map[string]config.ModelRoute{
		"nabu-live": {Primary: config.Target{Provider: "openai", Model: "gpt-live-1"}},
	})
	r.SetLiveProblems(o.problems)
	if o.callerKeys {
		r.SetCallerAdapter(func(name, key string) (provider.Adapter, bool) {
			return provider.NewOpenAIAdapter(name, upstreamURL, key, nil), name == "openai"
		})
	}
	tracker := usage.New(map[string]usage.Price{"openai/gpt-live-1": {PerMinute: 0.06}})
	enforcer := o.enforcer
	if enforcer == nil {
		enforcer = policy.New(nil, nil)
	}
	srv := New(r, enforcer, tracker, nil, discardLogger())
	if o.statePath != "" {
		if err := srv.SetLiveStateFile(o.statePath); err != nil {
			t.Fatal(err)
		}
	}
	return httptest.NewServer(srv.Handler()), tracker
}

func newLiveTestServer(t *testing.T, upstreamURL string) (*httptest.Server, *usage.Tracker) {
	return newLiveServer(t, upstreamURL, liveServerOpts{})
}

// liveVendor answers every session creation with one status and body.
func liveVendor(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// postLive posts a JSON body with optional header pairs and returns the status
// and body.
func postLive(t *testing.T, ts *httptest.Server, path, body string, headers ...string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func liveGet(t *testing.T, ts *httptest.Server, path, token string, out any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s: %d %s", path, resp.StatusCode, raw)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
}

func TestLiveSessionRewritesModelAndReturnsAnswer(t *testing.T) {
	up, got := liveUpstream(t)
	defer up.Close()
	ts, _ := newLiveTestServer(t, up.URL)
	defer ts.Close()

	body := `{"model":"nabu-live","session":{"instructions":"be brief","delegation":{"type":"client"}},"transport":{"type":"webrtc","sdp":"v=0 offer"}}`
	resp, err := http.Post(ts.URL+"/v1/live/sessions", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, raw)
	}
	if h := resp.Header.Get("X-Nabu-Model"); h != "gpt-live-1" {
		t.Fatalf("X-Nabu-Model = %q", h)
	}
	var answer struct {
		Session   struct{ ID string }
		Transport struct{ SDP string }
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if answer.Session.ID != "live_123" || answer.Transport.SDP != "v=0 answer" {
		t.Fatalf("answer not passed through: %+v", answer)
	}

	// The vendor saw its own model, the caller's instructions, the offer — and
	// not the gateway alias.
	var sent map[string]any
	if err := json.Unmarshal(*got, &sent); err != nil {
		t.Fatal(err)
	}
	if _, ok := sent["model"]; ok {
		t.Fatalf("alias leaked upstream: %s", *got)
	}
	session := sent["session"].(map[string]any)
	if session["model"] != "gpt-live-1" || session["instructions"] != "be brief" {
		t.Fatalf("session not rewritten: %v", session)
	}
	if sent["transport"].(map[string]any)["sdp"] != "v=0 offer" {
		t.Fatalf("offer not forwarded: %v", sent["transport"])
	}
}

func TestLiveUsageBillsSnapshotsOnce(t *testing.T) {
	up, _ := liveUpstream(t)
	defer up.Close()
	ts, tracker := newLiveTestServer(t, up.URL)
	defer ts.Close()

	create := `{"model":"nabu-live","transport":{"type":"webrtc","sdp":"o"}}`
	resp, err := http.Post(ts.URL+"/v1/live/sessions", "application/json", bytes.NewBufferString(create))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	report := func(body string) liveUsageResponse {
		t.Helper()
		resp, err := http.Post(ts.URL+"/v1/live/sessions/live_123/usage", "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(resp.Body)
			t.Fatalf("status %d: %s", resp.StatusCode, raw)
		}
		var out liveUsageResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	// 60 s at $0.06/min.
	first := report(`{"seconds":60}`)
	if first.CostUSD < 0.0599 || first.CostUSD > 0.0601 || first.BilledSeconds != 60 {
		t.Fatalf("first report: %+v", first)
	}
	// A repeated snapshot charges nothing more.
	again := report(`{"seconds":60}`)
	if again.CostUSD != 0 || again.BilledSeconds != 60 {
		t.Fatalf("repeat report billed again: %+v", again)
	}
	// An earlier snapshot arriving late charges nothing either.
	late := report(`{"seconds":30}`)
	if late.CostUSD != 0 || late.BilledSeconds != 60 {
		t.Fatalf("stale report billed: %+v", late)
	}
	// The final snapshot bills the remainder and closes the session.
	final := report(`{"seconds":90,"final":true}`)
	if final.CostUSD < 0.0299 || final.CostUSD > 0.0301 || final.BilledSeconds != 90 || !final.Final {
		t.Fatalf("final report: %+v", final)
	}
	resp, err = http.Post(ts.URL+"/v1/live/sessions/live_123/usage", "application/json", bytes.NewBufferString(`{"seconds":120}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("finalised session still accepted usage: %d", resp.StatusCode)
	}

	_, byModel := tracker.Snapshot()
	if got := byModel["openai/gpt-live-1"].CostUSD; got < 0.0899 || got > 0.0901 {
		t.Fatalf("tracker cost = %v, want 0.09", got)
	}
}

func TestLiveSessionUnknownAlias(t *testing.T) {
	up, _ := liveUpstream(t)
	defer up.Close()
	ts, _ := newLiveTestServer(t, up.URL)
	defer ts.Close()
	resp, err := http.Post(ts.URL+"/v1/live/sessions", "application/json", bytes.NewBufferString(`{"model":"nope","transport":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestARefusedLiveAliasAnswers503(t *testing.T) {
	up, got := liveUpstream(t)
	defer up.Close()
	ts, _ := newLiveServer(t, up.URL, liveServerOpts{problems: map[string]string{"nabu-live": "no per_minute price for openai/gpt-live-1"}})
	defer ts.Close()

	status, body := postLive(t, ts, "/v1/live/sessions", `{"model":"nabu-live","transport":{"type":"webrtc","sdp":"o"}}`)
	if status != http.StatusServiceUnavailable || !strings.Contains(body, "live alias misconfigured") {
		t.Fatalf("status %d: %s", status, body)
	}
	if len(*got) != 0 {
		t.Fatal("a refused alias reached the vendor")
	}
}

func TestAVendorRefusalKeepsItsClass(t *testing.T) {
	cases := []struct{ vendor, want int }{
		{http.StatusBadRequest, http.StatusBadRequest},
		{http.StatusUnprocessableEntity, http.StatusBadRequest},
		{http.StatusTooManyRequests, http.StatusTooManyRequests},
		// The gateway's key was refused: nothing the caller can fix.
		{http.StatusUnauthorized, http.StatusBadGateway},
		{http.StatusInternalServerError, http.StatusBadGateway},
	}
	for _, c := range cases {
		t.Run(fmt.Sprint(c.vendor), func(t *testing.T) {
			up := liveVendor(t, c.vendor, `{"error":{"message":"vendor says no"}}`)
			defer up.Close()
			ts, _ := newLiveTestServer(t, up.URL)
			defer ts.Close()

			status, body := postLive(t, ts, "/v1/live/sessions", `{"model":"nabu-live","transport":{"sdp":"o"}}`)
			if status != c.want || !strings.Contains(body, "vendor says no") {
				t.Fatalf("vendor %d answered as %d: %s", c.vendor, status, body)
			}
		})
	}
}

func TestLiveSessionsSurviveARestart(t *testing.T) {
	up, _ := liveUpstream(t)
	defer up.Close()
	path := filepath.Join(t.TempDir(), "live-sessions.json")

	before, _ := newLiveServer(t, up.URL, liveServerOpts{statePath: path})
	if status, body := postLive(t, before, "/v1/live/sessions", `{"model":"nabu-live","transport":{"sdp":"o"}}`); status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	if status, body := postLive(t, before, "/v1/live/sessions/live_123/usage", `{"seconds":60}`); status != http.StatusOK {
		t.Fatalf("usage: %d %s", status, body)
	}
	before.Close()

	// A redeploy: a new process on the same volume.
	after, tracker := newLiveServer(t, up.URL, liveServerOpts{statePath: path})
	defer after.Close()
	status, body := postLive(t, after, "/v1/live/sessions/live_123/usage", `{"seconds":90,"final":true}`)
	if status != http.StatusOK {
		t.Fatalf("usage after restart: %d %s — the rest of the call would go unbilled", status, body)
	}
	var out liveUsageResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	// Only the 30 seconds not billed before the restart, at $0.06/min.
	if out.BilledSeconds != 90 || out.CostUSD < 0.0299 || out.CostUSD > 0.0301 {
		t.Fatalf("after restart: %+v", out)
	}
	if _, byModel := tracker.Snapshot(); byModel["openai/gpt-live-1"].CostUSD > 0.0301 {
		t.Fatalf("the minute billed before the restart was billed again: %+v", byModel)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "live_123") {
		t.Fatalf("a finalised session is still on disk: %s", raw)
	}
}

func TestASessionOnTheCallersOwnKeyIsNeverBilled(t *testing.T) {
	up, _ := liveUpstream(t)
	defer up.Close()
	ts, tracker := newLiveServer(t, up.URL, liveServerOpts{callerKeys: true})
	defer ts.Close()

	if status, body := postLive(t, ts, "/v1/live/sessions", `{"model":"nabu-live","transport":{"sdp":"o"}}`,
		"X-Nabu-Key-openai", "sk-callers-own"); status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	// The usage report carries no key of its own — the product's server sends
	// it — and the vendor already bills the caller for every minute.
	status, body := postLive(t, ts, "/v1/live/sessions/live_123/usage", `{"seconds":120,"final":true}`)
	var out liveUsageResponse
	_ = json.Unmarshal([]byte(body), &out)
	if status != http.StatusOK || out.CostUSD != 0 {
		t.Fatalf("own-key session billed: %d %s", status, body)
	}
	if _, byModel := tracker.Snapshot(); byModel["openai/gpt-live-1"].CostUSD != 0 {
		t.Fatalf("tracker charged an own-key session: %+v", byModel)
	}
}

func TestHealthAndModelsListTheLiveAlias(t *testing.T) {
	up, _ := liveUpstream(t)
	defer up.Close()
	ts, _ := newLiveTestServer(t, up.URL)
	defer ts.Close()

	var health struct {
		Aliases []struct {
			ID       string `json:"id"`
			Kind     string `json:"kind"`
			Live     int    `json:"live_targets"`
			Disabled bool   `json:"disabled"`
		} `json:"aliases"`
	}
	liveGet(t, ts, "/v1/health", "", &health)
	found := false
	for _, a := range health.Aliases {
		if a.ID == "nabu-live" && a.Kind == "live" && a.Live == 1 && !a.Disabled {
			found = true
		}
	}
	if !found {
		t.Fatalf("nabu-live missing from /v1/health: %+v", health.Aliases)
	}

	var models struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	liveGet(t, ts, "/v1/models", "", &models)
	found = false
	for _, m := range models.Data {
		found = found || m.ID == "nabu-live"
	}
	if !found {
		t.Fatalf("nabu-live missing from /v1/models: %+v", models.Data)
	}
}

func TestModelsOfferTheLiveAliasOnlyToKeysAllowedIt(t *testing.T) {
	up, _ := liveUpstream(t)
	defer up.Close()
	enforcer := policy.New(nil, []policy.KeyConfig{
		{Key: "chat-only", Project: "c", Allow: []string{"nabu-fast"}},
		{Key: "voice", Project: "v", Allow: []string{"nabu-*"}},
	})
	ts, _ := newLiveServer(t, up.URL, liveServerOpts{enforcer: enforcer})
	defer ts.Close()

	lists := func(token string) bool {
		var models struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		liveGet(t, ts, "/v1/models", token, &models)
		for _, m := range models.Data {
			if m.ID == "nabu-live" {
				return true
			}
		}
		return false
	}
	if lists("chat-only") {
		t.Fatal("nabu-live offered to a key that may not use it")
	}
	if !lists("voice") {
		t.Fatal("nabu-live hidden from a key allowed nabu-*")
	}
}
