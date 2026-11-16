package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nabugate/internal/adminstore"
	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/router"
	"nabugate/internal/usage"
	"nabugate/internal/wsutil"
)

const geminiSetupBody = `{
  "setup": {
    "model": "models/gemini-2.5-flash-native-audio-preview-12-2025",
    "generationConfig": {"responseModalities": ["AUDIO"]},
    "tools": [{
      "functionDeclarations": [{
        "name": "search_products",
        "description": "Find products",
        "parameters": {"type": "object", "properties": {"q": {"type": "string"}}, "required": ["q"]},
        "behavior": "BLOCKING"
      }]
    }]
  }
}`

func geminiTokenUpstream(t *testing.T) (*httptest.Server, *[]byte) {
	t.Helper()
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1alpha/auth_tokens" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("x-goog-api-key") != "test-gemini-key" {
			t.Errorf("api key header = %q", r.Header.Get("x-goog-api-key"))
		}
		if strings.Contains(r.URL.RawQuery, "key=") {
			t.Error("api key leaked into the URL")
		}
		got, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"auth_tokens/test-token","expireTime":"2030-01-01T00:00:00Z"}`))
	}))
	return srv, &got
}

func newGeminiLiveServer(t *testing.T, upstream string, relay LiveRelay, enforcer *policy.Enforcer) (*httptest.Server, *usage.Tracker) {
	t.Helper()
	r := router.New(nil, nil, nil, nil, nil, nil, nil, discardLogger())
	r.SetLive(map[string]config.ModelRoute{
		"nabu-live": {Primary: config.Target{Provider: "openai", Model: "gpt-realtime-2.1"}},
	})
	tracker := usage.New(map[string]usage.Price{
		"gemini/gemini-2.5-flash-native-audio-preview-12-2025": {PerMinute: 0.06},
		"openai/gpt-realtime-2.1":                              {PerMinute: 0.30},
	})
	if enforcer == nil {
		enforcer = policy.New(nil, nil)
	}
	srv := New(r, enforcer, tracker, nil, discardLogger())
	relay.GeminiBase = upstream
	relay.GeminiKey = "test-gemini-key"
	srv.SetLiveRelay(relay)
	ts := httptest.NewServer(srv.Handler())
	return ts, tracker
}

func TestGeminiLiveTokenForwardsSetupAndBillsUsage(t *testing.T) {
	up, got := geminiTokenUpstream(t)
	defer up.Close()
	ts, tracker := newGeminiLiveServer(t, up.URL, LiveRelay{}, nil)
	defer ts.Close()

	status, body := postLive(t, ts, "/v1/live/gemini/token", geminiSetupBody)
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}
	var resp geminiTokenResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Token != "auth_tokens/test-token" || resp.Name != resp.Token {
		t.Fatalf("token = %+v", resp)
	}
	if !strings.Contains(resp.URL, "BidiGenerateContentConstrained") || !strings.Contains(resp.URL, "access_token=") {
		t.Fatalf("url = %s", resp.URL)
	}
	if !strings.Contains(resp.URL, "generativelanguage.googleapis.com") {
		t.Fatalf("direct url should name Google: %s", resp.URL)
	}
	if resp.SessionID == "" || resp.ExpiresAt == "" {
		t.Fatalf("response = %+v", resp)
	}
	for _, want := range []string{`"name":"search_products"`, `"description":"Find products"`, `"behavior":"BLOCKING"`, `"required":["q"]`} {
		if !strings.Contains(string(*got), want) {
			t.Fatalf("setup forwarded without %s\n%s", want, *got)
		}
	}
	if !strings.Contains(string(*got), `"bidiGenerateContentSetup"`) || strings.Contains(string(*got), "test-gemini-key") {
		t.Fatalf("token request = %s", *got)
	}

	status, body = postLive(t, ts, "/v1/live/sessions/"+resp.SessionID+"/usage", `{"seconds":120,"final":true}`)
	if status != http.StatusOK {
		t.Fatalf("usage %d: %s", status, body)
	}
	var usageResp liveUsageResponse
	if err := json.Unmarshal([]byte(body), &usageResp); err != nil {
		t.Fatal(err)
	}
	// 60s prepaid at mint, then the snapshot moves to 120s: 120s at $0.06/min.
	if usageResp.BilledSeconds != 120 || usageResp.CostUSD < 0.059 || usageResp.CostUSD > 0.061 {
		t.Fatalf("usage response = %+v", usageResp)
	}
	_, byModel := tracker.Snapshot()
	stat := byModel["gemini/gemini-2.5-flash-native-audio-preview-12-2025"]
	if stat.CostUSD < 0.119 || stat.CostUSD > 0.121 || stat.Requests != 1 {
		t.Fatalf("usage ledger = %+v", stat)
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/usage", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	ledger, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(ledger), "gemini-2.5-flash-native-audio-preview-12-2025") {
		t.Fatalf("GET /v1/usage %d: %s", res.StatusCode, ledger)
	}
}

func TestGeminiLiveTokenRequiresAuthBalanceAndAllowList(t *testing.T) {
	up, _ := geminiTokenUpstream(t)
	defer up.Close()
	enforcer := policy.New(nil, []policy.KeyConfig{{
		Key: "ng-limited", Project: "nabuchat", Allow: []string{"nabu-fast"}, RateLimit: 1,
	}})
	ts, _ := newGeminiLiveServer(t, up.URL, LiveRelay{}, enforcer)
	defer ts.Close()

	if status, body := postLive(t, ts, "/v1/live/gemini/token", geminiSetupBody); status != http.StatusUnauthorized {
		t.Fatalf("no key: %d %s", status, body)
	}
	if status, body := postLive(t, ts, "/v1/live/gemini/token", geminiSetupBody, "Authorization", "Bearer ng-limited"); status != http.StatusForbidden {
		t.Fatalf("allow-list: %d %s", status, body)
	}

	store, err := adminstore.Open(t.TempDir() + "/admin.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SignupUser("buyer@example.com", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	_, secret, err := store.NewToken("empty", []string{"nabu-*"}, 0, nil, "buyer@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	open := newGeminiLiveServerWithStore(t, up.URL, store)
	defer open.Close()
	if status, body := postLive(t, open, "/v1/live/gemini/token", geminiSetupBody, "Authorization", "Bearer "+secret); status != http.StatusPaymentRequired {
		t.Fatalf("empty balance: %d %s", status, body)
	}
}

func newGeminiLiveServerWithStore(t *testing.T, upstream string, store *adminstore.Store) *httptest.Server {
	t.Helper()
	r := router.New(nil, nil, nil, nil, nil, nil, nil, discardLogger())
	srv := New(r, policy.New(nil, nil), usage.New(nil), nil, discardLogger())
	srv.SetAdminStore(store)
	srv.SetLiveRelay(LiveRelay{GeminiBase: upstream, GeminiKey: "test-gemini-key"})
	return httptest.NewServer(srv.Handler())
}

func TestGeminiLiveTokenCapsOpenSessions(t *testing.T) {
	up, _ := geminiTokenUpstream(t)
	defer up.Close()
	ts, _ := newGeminiLiveServer(t, up.URL, LiveRelay{MaxSessions: 1}, nil)
	defer ts.Close()
	if status, body := postLive(t, ts, "/v1/live/gemini/token", geminiSetupBody); status != http.StatusOK {
		t.Fatalf("first: %d %s", status, body)
	}
	if status, body := postLive(t, ts, "/v1/live/gemini/token", geminiSetupBody); status != http.StatusTooManyRequests {
		t.Fatalf("second: %d %s", status, body)
	}
}

func TestLiveRelayForwardsToolFramesAndStopsAtTheCap(t *testing.T) {
	toolCall := []byte(`{"toolCall":{"functionCalls":[{"id":"c1","name":"search_products","args":{"q":"کفش","limit":2}}]}}`)
	toolResponse := []byte(`{"toolResponse":{"functionResponses":[{"id":"c1","name":"search_products","response":{"result":"ok","sku":"1"}}]}}`)

	var seen []byte
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1alpha/auth_tokens" {
			if r.Header.Get("x-goog-api-key") != "test-gemini-key" {
				t.Errorf("key header %q", r.Header.Get("x-goog-api-key"))
			}
			_, _ = w.Write([]byte(`{"name":"auth_tokens/relay-token","expireTime":"2030-01-01T00:00:00Z"}`))
			return
		}
		conn, err := wsutil.Accept(w, r)
		if err != nil {
			t.Errorf("upstream accept: %v", err)
			return
		}
		defer conn.Close()
		if !strings.Contains(r.URL.RawQuery, "access_token=") {
			t.Errorf("upstream url %s", r.URL.String())
		}
		op, payload, err := conn.Read()
		if err != nil {
			t.Errorf("upstream read: %v", err)
			return
		}
		if op != wsutil.OpText {
			t.Errorf("opcode %d", op)
		}
		seen = append([]byte(nil), payload...)
		if err := conn.Write(wsutil.OpText, toolCall); err != nil {
			t.Errorf("upstream write: %v", err)
		}
		_, _, _ = conn.Read()
	}))
	defer up.Close()

	ts, _ := newGeminiLiveServer(t, up.URL, LiveRelay{
		Enabled:        true,
		MaxSession:     30 * time.Minute,
		GeminiWSOrigin: up.URL,
	}, nil)
	defer ts.Close()

	if status, body := postLive(t, ts, "/v1/live/ws/session", `{"provider":"openai"}`); status != http.StatusServiceUnavailable {
		t.Fatalf("openai without a key: %d %s", status, body)
	}
	status, body := postLive(t, ts, "/v1/live/gemini/token", geminiSetupBody)
	if status != http.StatusOK {
		t.Fatalf("token %d: %s", status, body)
	}
	var resp geminiTokenResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.URL, "/v1/live/ws?ticket=") {
		t.Fatalf("relay url = %s", resp.URL)
	}
	if strings.Contains(resp.URL, "generativelanguage.googleapis.com") {
		t.Fatalf("relay url still points at Google: %s", resp.URL)
	}

	client, err := wsutil.Dial(t.Context(), resp.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	if err := client.Write(wsutil.OpText, toolResponse); err != nil {
		t.Fatal(err)
	}
	op, payload, err := client.Read()
	if err != nil {
		t.Fatal(err)
	}
	if op != wsutil.OpText || string(payload) != string(toolCall) {
		t.Fatalf("client got op %d %s", op, payload)
	}
	if string(seen) != string(toolResponse) {
		t.Fatalf("upstream got %s", seen)
	}
}

func TestLiveRelayDisabledDoesNotUpgrade(t *testing.T) {
	up, _ := geminiTokenUpstream(t)
	defer up.Close()
	ts, _ := newGeminiLiveServer(t, up.URL, LiveRelay{}, nil)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/live/ws?ticket=nope", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d %s", res.StatusCode, raw)
	}
	if status, body := postLive(t, ts, "/v1/live/ws/session", `{"provider":"openai","model":"nabu-live"}`); status != http.StatusNotFound {
		t.Fatalf("session route: %d %s", status, body)
	}
}

func TestLiveRelayClosesWhenTheMinuteCapIsReached(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1alpha/auth_tokens" {
			_, _ = w.Write([]byte(`{"name":"auth_tokens/short","expireTime":"2030-01-01T00:00:00Z"}`))
			return
		}
		conn, err := wsutil.Accept(w, r)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.Read()
	}))
	defer up.Close()
	ts, _ := newGeminiLiveServer(t, up.URL, LiveRelay{
		Enabled:        true,
		MaxSession:     200 * time.Millisecond,
		GeminiWSOrigin: up.URL,
	}, nil)
	defer ts.Close()
	status, body := postLive(t, ts, "/v1/live/gemini/token", geminiSetupBody)
	if status != http.StatusOK {
		t.Fatalf("token %d: %s", status, body)
	}
	var resp geminiTokenResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	client, err := wsutil.Dial(t.Context(), resp.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	op, _, err := client.Read()
	if err == nil && op != wsutil.OpClose {
		t.Fatal("relay stayed open past the cap")
	}
	// The session is finalised when the socket closes, so a late report is unknown.
	deadline := time.Now().Add(2 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		code, raw := postLive(t, ts, "/v1/live/sessions/"+resp.SessionID+"/usage", `{"seconds":10,"final":true}`)
		last = raw
		if code == http.StatusNotFound {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("session still open after the cap: %s", last)
}
