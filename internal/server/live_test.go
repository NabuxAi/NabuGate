package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

func newLiveTestServer(t *testing.T, upstreamURL string) (*httptest.Server, *usage.Tracker) {
	t.Helper()
	adapters := map[string]provider.Adapter{
		"openai": provider.NewOpenAIAdapter("openai", upstreamURL, "k", nil),
	}
	r := router.New(adapters, nil, nil, nil, nil, nil, nil, discardLogger())
	r.SetLive(map[string]config.ModelRoute{
		"nabu-live": {Primary: config.Target{Provider: "openai", Model: "gpt-live-1"}},
	})
	tracker := usage.New(map[string]usage.Price{"openai/gpt-live-1": {PerMinute: 0.06}})
	srv := New(r, policy.New(nil, nil), tracker, nil, discardLogger())
	return httptest.NewServer(srv.Handler()), tracker
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
