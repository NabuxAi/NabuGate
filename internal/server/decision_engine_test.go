package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// setupDecisionEngineTest wires a chat alias ("nabu-fast") to an OpenAI-wire
// fake that answers chat/completions with top logprobs and 404s the native
// /alpha/decisions path, so the local engine is the only thing that can serve.
func setupDecisionEngineTest(t *testing.T) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/alpha/decisions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"content": "B"}, "logprobs": {"content": [{"top_logprobs": [
				{"token": "A", "logprob": -3.5},
				{"token": "B", "logprob": -0.05},
				{"token": "C", "logprob": -5.0}
			]}]}}],
			"usage": {"prompt_tokens": 11, "completion_tokens": 1}
		}`))
	}))
	t.Cleanup(upstream.Close)

	adapters := map[string]provider.Adapter{
		"groq": provider.NewOpenAIAdapter("groq", upstream.URL, "k", nil),
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := router.New(adapters, map[string]config.ModelRoute{
		"nabu-fast": {Primary: config.Target{Provider: "groq", Model: "llama-test"}},
	}, nil, nil, nil, nil, nil, logger)

	enforcer := policy.New([]string{"valid-key"}, []policy.KeyConfig{
		{Key: "play-key", Project: "playground", Allow: []string{"nabu-fast"}},
	})
	tracker := usage.New(nil)
	srv := New(r, enforcer, tracker, nil, logger)
	gw := httptest.NewServer(srv.Handler())
	t.Cleanup(gw.Close)
	return gw
}

func postDecision(t *testing.T, url string, body any) (*http.Response, map[string]any) {
	t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer play-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestDecisionOnChatAliasWithLocalEngine(t *testing.T) {
	gw := setupDecisionEngineTest(t)

	resp, out := postDecision(t, gw.URL+"/v1/decisions", map[string]any{
		"model":  "nabu-fast",
		"engine": "local",
		"state":  "What color is the sky?",
		"questions": map[string]any{
			"color": map[string]any{
				"type":     "choice",
				"criteria": map[string]string{"red": "stop signs", "blue": "sky", "green": "grass"},
			},
		},
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", resp.StatusCode, out)
	}
	if out["engine"] != "local" {
		t.Errorf("expected engine local, got %v", out["engine"])
	}
	answers := out["answers"].(map[string]any)["color"].(map[string]any)
	// Sorted criteria letters: blue→A (-3.5), green→B (-0.05), red→C (-5).
	if answers["choice"] != "green" {
		t.Errorf("expected choice green, got %v", answers["choice"])
	}
	probs := answers["probabilities"].(map[string]any)
	if len(probs) != 3 {
		t.Fatalf("expected 3 option probabilities, got %v", probs)
	}
	if p := probs["green"].(float64); p < 0.9 || p > 1.0 {
		t.Errorf("winner probability %f out of band", p)
	}
}

// Engine auto must try the native path first and, when the provider has none
// (404), still answer through the local engine on the same rung.
func TestDecisionEngineAutoFallsBackToLocal(t *testing.T) {
	gw := setupDecisionEngineTest(t)

	resp, out := postDecision(t, gw.URL+"/v1/systemone", map[string]any{
		"model": "nabu-fast",
		"state": "Is the statement true: Paris is the capital of France?",
		"questions": map[string]any{
			"fact": map[string]any{"type": "noul"},
		},
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", resp.StatusCode, out)
	}
	if out["engine"] != "local" {
		t.Errorf("expected fallback to local engine, got %v", out["engine"])
	}
	answers := out["answers"].(map[string]any)["fact"].(map[string]any)
	noul, ok := answers["noul"].(float64)
	if !ok {
		t.Fatalf("noul answer missing: %v", answers)
	}
	// The fake puts A (true) at -3.5 and B (false) at -0.05, so the verdict
	// lands on false with p_true = e^-3.5/(e^-3.5+e^-0.05) ≈ 0.03. What the
	// test cares about is that a local answer came back at all.
	if noul > 0.1 {
		t.Errorf("expected false to win under these logprobs, noul = %f", noul)
	}
}

func TestDecisionCalibrateEndpoint(t *testing.T) {
	gw := setupDecisionEngineTest(t)

	resp, out := postDecision(t, gw.URL+"/v1/decisions/calibrate", map[string]any{
		"model":  "nabu-fast",
		"engine": "local",
		"samples": []map[string]any{
			{
				"state":     "sky",
				"questions": map[string]any{"c": map[string]any{"type": "choice", "criteria": map[string]string{"red": "r", "blue": "b"}}},
				"expected":  map[string]string{"c": "blue"},
			},
			{
				"state":     "fire",
				"questions": map[string]any{"c": map[string]any{"type": "choice", "criteria": map[string]string{"red": "r", "blue": "b"}}},
				"expected":  map[string]string{"c": "blue"},
			},
		},
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", resp.StatusCode, out)
	}
	if temp, ok := out["temperature"].(float64); !ok || temp < 1 {
		t.Errorf("expected temperature >= 1, got %v", out["temperature"])
	}
	if len(out["bins_before"].([]any)) != 10 || len(out["bins_after"].([]any)) != 10 {
		t.Errorf("expected 10 reliability bins before and after")
	}
	if n, ok := out["points"].(float64); !ok || n != 2 {
		t.Errorf("expected 2 labelled points, got %v", out["points"])
	}
}

func TestDecisionEngineValidation(t *testing.T) {
	gw := setupDecisionEngineTest(t)

	resp, _ := postDecision(t, gw.URL+"/v1/decisions", map[string]any{
		"model": "nabu-fast", "engine": "quantum",
		"state": "s", "questions": map[string]any{"q": map[string]any{"type": "noul"}},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad engine value: expected 400, got %d", resp.StatusCode)
	}

	// Unknown alias on an unrestricted key reaches the router, which answers
	// "unknown decision alias" → 400 (an allow-listed key would 403 first).
	req3, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/decisions",
		bytes.NewReader([]byte(`{"model":"no-such-alias","state":"s","questions":{"q":{"type":"noul"}}}`)))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Authorization", "Bearer valid-key")
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown alias: expected 400, got %d", resp3.StatusCode)
	}
}
