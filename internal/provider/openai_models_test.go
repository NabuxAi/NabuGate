package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestOpenAIListModelsDataShape verifies the canonical {"data":[{"id":…}]}
// response is parsed, empty IDs are dropped, and the /models path is hit.
func TestOpenAIListModelsDataShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path = %q, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("auth header = %q", got)
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"openai/gpt-5.5"},{"id":"google/gemini-2.5-flash"},{"id":""}]}`))
	}))
	defer srv.Close()

	a := NewOpenAIAdapter("parspack", srv.URL, "k", nil)
	ids, err := a.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "openai/gpt-5.5" || ids[1] != "google/gemini-2.5-flash" {
		t.Fatalf("ids = %v", ids)
	}
}

// TestOpenAIListModelsBareArray verifies the bare-array-of-objects fallback used
// by some OpenAI-wire aggregators.
func TestOpenAIListModelsBareArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"a/one"},{"id":"b/two"}]`))
	}))
	defer srv.Close()

	a := NewOpenAIAdapter("p", srv.URL, "k", nil)
	ids, err := a.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "a/one" || ids[1] != "b/two" {
		t.Fatalf("ids = %v", ids)
	}
}

// TestOpenAIListModelsError verifies an upstream 4xx surfaces as an error.
func TestOpenAIListModelsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()

	a := NewOpenAIAdapter("p", srv.URL, "k", nil)
	if _, err := a.ListModels(context.Background()); err == nil {
		t.Fatal("expected error on 401")
	}
}

// TestOpenAIResponsesProxy verifies the Responses proxy forwards the body to
// /responses verbatim and returns the raw upstream response for streaming back.
func TestOpenAIResponsesProxy(t *testing.T) {
	var gotBody map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("path = %q, want /responses", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","status":"completed"}`))
	}))
	defer srv.Close()

	a := NewOpenAIAdapter("parspack", srv.URL, "k", nil)
	resp, err := a.Responses(context.Background(), []byte(`{"model":"openai/gpt-5.5","input":"hi","reasoning":{"effort":"medium"}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	if string(raw) != `{"id":"resp_1","status":"completed"}` {
		t.Fatalf("body = %s", raw)
	}
	var gotModel string
	_ = json.Unmarshal(gotBody["model"], &gotModel)
	if gotModel != "openai/gpt-5.5" {
		t.Fatalf("model forwarded = %q", gotModel)
	}
	if _, ok := gotBody["reasoning"]; !ok {
		t.Fatal("reasoning param not forwarded to upstream")
	}
}
