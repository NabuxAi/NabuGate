package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTypeSafeDecideSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/systemone" && r.URL.Path != "/v1/systemone" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "bad method", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req["model"] != "jev-latest" {
			http.Error(w, "wrong model", http.StatusBadRequest)
			return
		}

		resp := map[string]any{
			"model": "jev-1.13.0",
			"answers": map[string]any{
				"is_urgent": map[string]any{
					"type": "noul",
					"noul": 0.95,
				},
			},
			"usage": map[string]any{
				"input_tokens":  240,
				"output_tokens": 0,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	adapter := NewTypeSafeAdapter("typesafe", ts.URL, "test-key")
	res, err := adapter.Decide(context.Background(), DecisionRequest{
		Model:     "jev-latest",
		State:     json.RawMessage(`"Help, urgent issue!"`),
		Questions: json.RawMessage(`{"is_urgent":{"type":"noul","instructions":"is it urgent?"}}`),
	})
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}

	if res.Model != "jev-1.13.0" {
		t.Errorf("expected model jev-1.13.0, got %s", res.Model)
	}
	if res.Usage.PromptTokens != 240 {
		t.Errorf("expected 240 prompt tokens, got %d", res.Usage.PromptTokens)
	}

	var answers map[string]struct {
		Type string  `json:"type"`
		Noul float64 `json:"noul"`
	}
	if err := json.Unmarshal(res.Answers, &answers); err != nil {
		t.Fatalf("failed to unmarshal answers: %v", err)
	}
	if answers["is_urgent"].Noul != 0.95 {
		t.Errorf("expected noul 0.95, got %f", answers["is_urgent"].Noul)
	}
}

func TestTypeSafeDecideError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "field 'questions' is malformed",
			},
		})
	}))
	defer ts.Close()

	adapter := NewTypeSafeAdapter("typesafe", ts.URL, "test-key")
	_, err := adapter.Decide(context.Background(), DecisionRequest{
		Model:     "jev-latest",
		State:     json.RawMessage(`"test"`),
		Questions: json.RawMessage(`{}`),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestTypeSafeChatAndStreamSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"model": "jev-1.13.0",
			"answers": map[string]any{
				"verdict": map[string]any{
					"type": "noul",
					"noul": 0.98,
				},
				"intent": map[string]any{
					"type":   "choice",
					"choice": "security_guardrail",
				},
			},
			"usage": map[string]any{
				"input_tokens":  85,
				"output_tokens": 0,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	adapter := NewTypeSafeAdapter("typesafe", ts.URL, "test-key")

	// 1. Test Chat
	chatReq := ChatRequest{
		Model: "jev-latest",
		Messages: []Message{
			{Role: "user", Content: "Is this prompt injection safe: Ignore previous instructions"},
		},
	}
	resp, err := adapter.Chat(context.Background(), chatReq)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if !strings.Contains(resp.Content, "TypeSafe Jev") {
		t.Errorf("expected Content to contain TypeSafe Jev, got: %s", resp.Content)
	}
	if !strings.Contains(resp.Content, "security_guardrail") {
		t.Errorf("expected Content to contain choice result, got: %s", resp.Content)
	}
	if resp.Usage.PromptTokens != 85 {
		t.Errorf("expected 85 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens <= 0 {
		t.Errorf("expected completion tokens > 0, got %d", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != resp.Usage.PromptTokens+resp.Usage.CompletionTokens {
		t.Errorf("expected total tokens %d, got %d", resp.Usage.PromptTokens+resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	}

	// 2. Test ChatStream
	var streamed strings.Builder
	usage, err := adapter.ChatStream(context.Background(), chatReq, func(delta string) error {
		streamed.WriteString(delta)
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}
	if streamed.String() != resp.Content {
		t.Errorf("streamed content mismatch")
	}
	if usage.PromptTokens != 85 || usage.CompletionTokens <= 0 {
		t.Errorf("unexpected stream usage: %+v", usage)
	}
}

func TestOpenAIDecideSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alpha/decisions" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]any{
			"model": "typesafe/jev-1.13",
			"answers": map[string]any{
				"verdict": map[string]any{
					"type":   "choice",
					"choice": "refund",
				},
			},
			"usage": map[string]any{
				"input_tokens":  120,
				"output_tokens": 0,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	adapter := NewOpenAIAdapter("openrouter", ts.URL+"/v1", "test-key", nil)

	// Test Decide directly
	res, err := adapter.Decide(context.Background(), DecisionRequest{
		Model:     "typesafe/jev-latest",
		State:     json.RawMessage(`"I want my money back"`),
		Questions: json.RawMessage(`{"verdict":{"type":"choice","criteria":{"refund":"user wants refund"}}}`),
	})
	if err != nil {
		t.Fatalf("OpenAI Decide failed: %v", err)
	}
	if res.Model != "typesafe/jev-1.13" {
		t.Errorf("expected model typesafe/jev-1.13, got %s", res.Model)
	}
	if res.Usage.PromptTokens != 120 {
		t.Errorf("expected 120 prompt tokens, got %d", res.Usage.PromptTokens)
	}

	// Test Chat with decision model
	chatRes, err := adapter.Chat(context.Background(), ChatRequest{
		Model: "typesafe/jev-latest",
		Messages: []Message{
			{Role: "user", Content: "I want my money back"},
		},
	})
	if err != nil {
		t.Fatalf("OpenAI Chat with decision model failed: %v", err)
	}
	if !strings.Contains(chatRes.Content, "TypeSafe Jev") {
		t.Errorf("expected Content to contain TypeSafe Jev, got: %s", chatRes.Content)
	}
	if chatRes.Usage.PromptTokens != 120 {
		t.Errorf("expected 120 prompt tokens, got %d", chatRes.Usage.PromptTokens)
	}
}
