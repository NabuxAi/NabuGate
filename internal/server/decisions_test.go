package server

import (
	"bufio"
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
