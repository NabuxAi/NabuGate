package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

func setupDecisionTestEnvironment(t *testing.T) (*httptest.Server, *httptest.Server, *usage.Tracker) {
	upstreamTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/systemone") || strings.HasSuffix(r.URL.Path, "/decisions") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"model": "jev-latest",
				"answers": {
					"intent": {
						"type": "choice",
						"choice": "security_guardrail",
						"confidence": 0.98
					},
					"verdict": {
						"type": "noul",
						"noul": 0.95
					},
					"severity": {
						"type": "score",
						"score": 3.0,
						"confidence": 0.92,
						"legend": {"3": "Critical"}
					}
				},
				"usage": {
					"input_tokens": 120,
					"output_tokens": 0
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))

	tsAdapter := provider.NewTypeSafeAdapter("typesafe", upstreamTS.URL, "test-api-key")
	adapters := map[string]provider.Adapter{
		"typesafe": tsAdapter,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := router.New(adapters, nil, nil, nil, nil, nil, nil, logger)
	r.SetDecisions(map[string]config.ModelRoute{
		"nabu-decision": {
			Primary: config.Target{Provider: "typesafe", Model: "jev-latest"},
		},
	})

	enforcer := policy.New([]string{"valid-key"}, []policy.KeyConfig{
		{Key: "proj-key", Project: "crm", Allow: []string{"nabu-decision"}},
	})
	tracker := usage.New(map[string]usage.Price{
		"typesafe/jev-latest": {Input: 0.042, Output: 0},
	})
	srv := New(r, enforcer, tracker, nil, logger)
	gw := httptest.NewServer(srv.Handler())
	return gw, upstreamTS, tracker
}

func TestServerDecisionsEndpoint(t *testing.T) {
	ts, upstream, tracker := setupDecisionTestEnvironment(t)
	defer ts.Close()
	defer upstream.Close()

	endpoints := []string{"/v1/systemone", "/v1/decisions"}
	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			body := map[string]any{
				"model":     "nabu-decision",
				"state":     "Customer is locked out of account",
				"questions": map[string]any{"is_urgent": map[string]any{"type": "noul", "instructions": "is urgent?"}},
			}
			data, _ := json.Marshal(body)

			req, _ := http.NewRequest(http.MethodPost, ts.URL+ep, bytes.NewReader(data))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer proj-key")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				raw, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected 200, got %d: %s", resp.StatusCode, raw)
			}

			if prov := resp.Header.Get("X-Nabu-Provider"); prov != "typesafe" {
				t.Errorf("expected X-Nabu-Provider typesafe, got %s", prov)
			}
			if model := resp.Header.Get("X-Nabu-Model"); model != "jev-latest" {
				t.Errorf("expected X-Nabu-Model jev-latest, got %s", model)
			}

			var res struct {
				Model   string `json:"model"`
				Answers map[string]struct {
					Type string  `json:"type"`
					Noul float64 `json:"noul"`
				} `json:"answers"`
				Usage struct {
					InputTokens int `json:"input_tokens"`
					TotalTokens int `json:"total_tokens"`
				} `json:"usage"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if res.Answers["verdict"].Noul != 0.95 {
				t.Errorf("expected verdict noul 0.95, got %f", res.Answers["verdict"].Noul)
			}
			if res.Usage.InputTokens != 120 {
				t.Errorf("expected 120 input tokens, got %d", res.Usage.InputTokens)
			}
		})
	}

	byProj, _ := tracker.Snapshot()
	if stat, ok := byProj["crm"]; !ok || stat.Requests == 0 {
		t.Errorf("expected usage recorded for project crm")
	}
}

// nabu-v1m is the v1m.ir-fronted decision alias riding the same typesafe
// adapter. The upstream below answers with the recorded shape of a live v1m
// call (2026-10-11): answers carry fields the plain TypeSafe shape does not
// (confidence, probabilities, legend), and the questions contract is v1m's
// own — instructions on every question, criteria instead of choices/min-max.
// Everything must reach the caller untouched, and the zero-priced route must
// still meter its tokens.
func TestServerDecisionsV1MAlias(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/systemone" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer v1m-key" {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req["model"] != "v1m-latest" {
			http.Error(w, "wrong model", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "v1m-1.0",
			"answers": {
				"is_fraud": {"type": "noul", "noul": 0.88},
				"action": {"type": "choice", "choice": "manual_review", "confidence": 0.81, "probabilities": {"manual_review": 0.81, "auto_approve": 0.19}},
				"risk": {"type": "score", "score": 3.5, "confidence": 0.77, "legend": {"3": "high", "4": "critical"}, "probabilities": {"3": 0.4, "4": 0.6}}
			},
			"usage": {"input_tokens": 312, "output_tokens": 48}
		}`))
	}))
	defer upstream.Close()

	adapters := map[string]provider.Adapter{
		"v1m": provider.NewTypeSafeAdapter("v1m", upstream.URL, "v1m-key"),
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := router.New(adapters, nil, nil, nil, nil, nil, nil, logger)
	r.SetDecisions(map[string]config.ModelRoute{
		"nabu-v1m": {
			Primary: config.Target{Provider: "v1m", Model: "v1m-latest"},
			Fallback: []config.Target{
				{Provider: "typesafe", Model: "jev-latest"},
			},
		},
	})

	enforcer := policy.New([]string{"valid-key"}, []policy.KeyConfig{
		{Key: "proj-key", Project: "crm", Allow: []string{"nabu-v1m"}},
	})
	tracker := usage.New(map[string]usage.Price{
		"v1m/v1m-latest": {Input: 0, Output: 0},
	})
	srv := New(r, enforcer, tracker, nil, logger)
	gw := httptest.NewServer(srv.Handler())
	defer gw.Close()

	body := map[string]any{
		"model": "nabu-v1m",
		"state": map[string]any{"amount": 9900, "country": "IR"},
		"questions": map[string]any{
			"is_fraud": map[string]any{"type": "noul", "instructions": "Is this transaction fraudulent?"},
			"action": map[string]any{
				"type":         "choice",
				"instructions": "What should we do?",
				"criteria":     map[string]string{"manual_review": "a human looks", "auto_approve": "let it through"},
			},
			"risk": map[string]any{
				"type":         "score",
				"instructions": "How risky is this?",
				"criteria":     []string{"low", "moderate", "high", "critical"},
			},
		},
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/systemone", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer proj-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, raw)
	}

	if prov := resp.Header.Get("X-Nabu-Provider"); prov != "v1m" {
		t.Errorf("expected X-Nabu-Provider v1m, got %s", prov)
	}
	if model := resp.Header.Get("X-Nabu-Model"); model != "v1m-latest" {
		t.Errorf("expected X-Nabu-Model v1m-latest, got %s", model)
	}

	var res struct {
		Model   string `json:"model"`
		Answers struct {
			IsFraud struct {
				Type string  `json:"type"`
				Noul float64 `json:"noul"`
			} `json:"is_fraud"`
			Action struct {
				Choice        string             `json:"choice"`
				Confidence    float64            `json:"confidence"`
				Probabilities map[string]float64 `json:"probabilities"`
			} `json:"action"`
			Risk struct {
				Score         float64            `json:"score"`
				Legend        map[string]string  `json:"legend"`
				Probabilities map[string]float64 `json:"probabilities"`
			} `json:"risk"`
		} `json:"answers"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if res.Model != "v1m-latest" {
		t.Errorf("expected model v1m-latest, got %s", res.Model)
	}
	if res.Answers.IsFraud.Noul != 0.88 {
		t.Errorf("expected is_fraud noul 0.88, got %f", res.Answers.IsFraud.Noul)
	}
	if res.Answers.Action.Choice != "manual_review" || res.Answers.Action.Confidence != 0.81 {
		t.Errorf("unexpected action answer: %+v", res.Answers.Action)
	}
	if res.Answers.Action.Probabilities["auto_approve"] != 0.19 {
		t.Errorf("expected action probabilities to pass through, got %v", res.Answers.Action.Probabilities)
	}
	if res.Answers.Risk.Legend["4"] != "critical" || res.Answers.Risk.Probabilities["3"] != 0.4 {
		t.Errorf("expected risk legend/probabilities to pass through, got %v / %v", res.Answers.Risk.Legend, res.Answers.Risk.Probabilities)
	}
	if res.Usage.InputTokens != 312 || res.Usage.OutputTokens != 48 || res.Usage.TotalTokens != 360 {
		t.Errorf("unexpected usage: %+v", res.Usage)
	}

	// The explicit-zero price meters no cost, but the tokens and the request
	// must still land on the project's ledger.
	byProj, _ := tracker.Snapshot()
	stat, ok := byProj["crm"]
	if !ok || stat.Requests == 0 {
		t.Fatalf("expected usage recorded for project crm")
	}
	if stat.PromptTokens != 312 || stat.CompletionTokens != 48 {
		t.Errorf("expected 312/48 tracked tokens, got %d/%d", stat.PromptTokens, stat.CompletionTokens)
	}
	if stat.CostUSD != 0 {
		t.Errorf("expected zero cost on the free quota price, got %f", stat.CostUSD)
	}
}

func TestServerDecisionsValidation(t *testing.T) {
	ts, upstream, _ := setupDecisionTestEnvironment(t)
	defer ts.Close()
	defer upstream.Close()

	cases := []struct {
		name string
		body string
		want int
	}{
		{"missing model", `{"state":"abc","questions":{"q":1}}`, http.StatusBadRequest},
		{"missing state", `{"model":"nabu-decision","questions":{"q":1}}`, http.StatusBadRequest},
		{"missing questions", `{"model":"nabu-decision","state":"abc"}`, http.StatusBadRequest},
		{"unauthorized", `{"model":"nabu-decision","state":"abc","questions":{"q":1}}`, http.StatusUnauthorized},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/systemone", bytes.NewReader([]byte(c.body)))
			req.Header.Set("Content-Type", "application/json")
			if c.name != "unauthorized" {
				req.Header.Set("Authorization", "Bearer proj-key")
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != c.want {
				t.Errorf("expected status %d, got %d", c.want, resp.StatusCode)
			}
		})
	}
}

func TestServerChatWithDecisionModel(t *testing.T) {
	ts, upstream, tracker := setupDecisionTestEnvironment(t)
	defer ts.Close()
	defer upstream.Close()

	chatBody := map[string]any{
		"model": "nabu-decision",
		"messages": []map[string]string{
			{"role": "user", "content": "Please verify this user payload for security exploits: drop database;"},
		},
	}
	data, _ := json.Marshal(chatBody)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer proj-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, raw)
	}

	if prov := resp.Header.Get("X-Nabu-Provider"); prov != "typesafe" {
		t.Errorf("expected X-Nabu-Provider typesafe, got %s", prov)
	}
	if model := resp.Header.Get("X-Nabu-Model"); model != "jev-latest" {
		t.Errorf("expected X-Nabu-Model jev-latest, got %s", model)
	}

	var res struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode chat completion response: %v", err)
	}

	if len(res.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
	content := res.Choices[0].Message.Content
	if !strings.Contains(content, "TypeSafe Jev") {
		t.Errorf("expected content to contain 'TypeSafe Jev', got: %s", content)
	}
	if !strings.Contains(content, "security_guardrail") {
		t.Errorf("expected content to contain 'security_guardrail', got: %s", content)
	}
	if !strings.Contains(content, "Critical") {
		t.Errorf("expected content to contain 'Critical', got: %s", content)
	}

	if res.Usage.PromptTokens != 120 {
		t.Errorf("expected 120 prompt tokens, got %d", res.Usage.PromptTokens)
	}
	if res.Usage.CompletionTokens <= 0 {
		t.Errorf("expected positive completion tokens, got %d", res.Usage.CompletionTokens)
	}
	if res.Usage.TotalTokens != res.Usage.PromptTokens+res.Usage.CompletionTokens {
		t.Errorf("expected total tokens %d, got %d", res.Usage.PromptTokens+res.Usage.CompletionTokens, res.Usage.TotalTokens)
	}

	// Verify usage tracking
	byProj, _ := tracker.Snapshot()
	stat, ok := byProj["crm"]
	if !ok || stat.Requests == 0 {
		t.Fatalf("expected usage recorded for project crm")
	}
	if stat.PromptTokens != 120 {
		t.Errorf("expected 120 tracked prompt tokens, got %d", stat.PromptTokens)
	}
	if stat.CostUSD <= 0 {
		t.Errorf("expected tracked cost > 0, got %f", stat.CostUSD)
	}
}

func TestServerChatStreamWithDecisionModel(t *testing.T) {
	ts, upstream, tracker := setupDecisionTestEnvironment(t)
	defer ts.Close()
	defer upstream.Close()

	chatBody := map[string]any{
		"model":  "nabu-decision",
		"stream": true,
		"messages": []map[string]string{
			{"role": "user", "content": "Analyze intent and sentiment"},
		},
	}
	data, _ := json.Marshal(chatBody)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer proj-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, raw)
	}

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("expected text/event-stream content-type, got %s", ct)
	}

	scanner := bufio.NewScanner(resp.Body)
	var fullContent strings.Builder
	var receivedDone bool

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			receivedDone = true
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			t.Fatalf("failed to decode chunk JSON: %v, line: %s", err, payload)
		}
		if len(chunk.Choices) > 0 {
			fullContent.WriteString(chunk.Choices[0].Delta.Content)
		}
	}

	if !receivedDone {
		t.Errorf("expected to receive [DONE] marker in SSE stream")
	}

	allText := fullContent.String()
	if !strings.Contains(allText, "TypeSafe Jev") {
		t.Errorf("expected streamed text to contain 'TypeSafe Jev', got: %s", allText)
	}
	if !strings.Contains(allText, "security_guardrail") {
		t.Errorf("expected streamed text to contain 'security_guardrail', got: %s", allText)
	}

	// Verify usage tracking on stream completion
	byProj, _ := tracker.Snapshot()
	stat, ok := byProj["crm"]
	if !ok || stat.Requests == 0 {
		t.Fatalf("expected stream usage recorded for project crm")
	}
	if stat.PromptTokens != 120 {
		t.Errorf("expected 120 tracked prompt tokens, got %d", stat.PromptTokens)
	}
}

// ───────────────────── cost visibility on /v1/decisions ─────────────────────

// The decision response now carries what the caller was charged — on the
// X-Nabu-Cost-USD header like every other metered route, and additively in
// the usage object — and an upstream that answered without a usage object is
// metered as a flagged estimate instead of an indifferent zero.
func TestDecisionsCostIsOnTheWireAndTheBooks(t *testing.T) {
	ts, _, _ := setupDecisionTestEnvironment(t)
	defer ts.Close()

	body := `{"model":"nabu-decision","state":{"amount":9900},"questions":{"intent":{"type":"choice"}}}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/decisions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer proj-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	header := resp.Header.Get("X-Nabu-Cost-USD")
	if header == "" {
		t.Fatal("X-Nabu-Cost-USD missing from the decision response")
	}
	var out struct {
		Usage struct {
			CostUSD     float64 `json:"cost_usd"`
			InputTokens int     `json:"input_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	// The upstream reported input_tokens=120 at $0.042 per million — a number
	// in the fourth decimal, which is why the header carries six.
	if got := strconv.FormatFloat(out.Usage.CostUSD, 'f', 6, 64); got != header {
		t.Fatalf("body cost %s disagrees with header %s", got, header)
	}
	if want := 120.0 / 1e6 * 0.042; math.Abs(out.Usage.CostUSD-want) > 1e-9 {
		t.Errorf("cost_usd = %v, want %v (120 input tokens at the priced rate)", out.Usage.CostUSD, want)
	}
}

// An upstream that gives answers but no usage at all: the request is still
// one estimated request on the books, flagged, and the cost header still
// shows what the estimate was charged.
func TestDecisionsWithoutUpstreamUsageIsEstimated(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"jev-latest","answers":{"intent":{"type":"choice","choice":"general_chat"}}}`))
	}))
	defer upstream.Close()

	adapters := map[string]provider.Adapter{
		"typesafe": provider.NewTypeSafeAdapter("typesafe", upstream.URL, "k"),
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := router.New(adapters, nil, nil, nil, nil, nil, nil, logger)
	r.SetDecisions(map[string]config.ModelRoute{
		"nabu-decision": {Primary: config.Target{Provider: "typesafe", Model: "jev-latest"}},
	})
	tracker := usage.New(map[string]usage.Price{"typesafe/jev-latest": {Input: 0.042, Output: 0}})
	srv := New(r, policy.New(nil, nil), tracker, nil, logger)
	gw := httptest.NewServer(srv.Handler())
	defer gw.Close()

	body := `{"model":"nabu-decision","state":{"amount":9900},"questions":{"intent":{"type":"choice"}}}`
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/decisions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var out struct {
		Usage struct {
			CostUSD   float64 `json:"cost_usd"`
			Estimated *bool   `json:"estimated"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Usage.Estimated == nil || !*out.Usage.Estimated {
		t.Error("the response must say the counts are an estimate")
	}
	if out.Usage.CostUSD <= 0 {
		t.Errorf("cost_usd = %v, want the estimate charged at the priced rate", out.Usage.CostUSD)
	}
	if resp.Header.Get("X-Nabu-Cost-USD") == "" {
		t.Error("X-Nabu-Cost-USD missing")
	}
	_, byModel := tracker.Snapshot()
	row := byModel["typesafe/jev-latest"]
	if row.EstimatedRequests != 1 || row.PromptTokens <= 0 {
		t.Errorf("books = %+v, want one estimated request with positive tokens", row)
	}
}
