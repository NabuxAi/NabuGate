package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The OpenAI embeddings API takes a "dimensions" parameter for models that can
// emit more than one vector width. Callers that index the result depend on it:
// a store with a fixed-width column cannot accept whatever the provider default
// happens to be, so silently dropping the field corrupts the index.

func TestOpenAIEmbedForwardsDimensions(t *testing.T) {
	var got map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1,0.2]}]}`))
	}))
	defer srv.Close()

	adapter := NewOpenAIAdapter("test", srv.URL, "key", nil)
	dims := 1536

	if _, err := adapter.Embed(context.Background(), EmbeddingRequest{
		Model:      "text-embedding-3-small",
		Input:      []string{"سلام"},
		Dimensions: &dims,
	}); err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if got["dimensions"] != float64(1536) {
		t.Errorf("dimensions = %v, want 1536", got["dimensions"])
	}
}

func TestOpenAIEmbedOmitsDimensionsWhenUnset(t *testing.T) {
	var got map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1]}]}`))
	}))
	defer srv.Close()

	adapter := NewOpenAIAdapter("test", srv.URL, "key", nil)
	if _, err := adapter.Embed(context.Background(), EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []string{"hi"},
	}); err != nil {
		t.Fatalf("Embed: %v", err)
	}

	// Providers that do not support the field reject it outright, so an unset
	// request must not carry it at all.
	if _, present := got["dimensions"]; present {
		t.Error("dimensions was sent even though the caller left it unset")
	}
}

func TestGeminiEmbedMapsDimensionsPerRequest(t *testing.T) {
	var got struct {
		Requests []map[string]any `json:"requests"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[{"values":[0.1]},{"values":[0.2]}]}`))
	}))
	defer srv.Close()

	adapter := NewGeminiAdapter("gemini", srv.URL, "key")
	dims := 1536

	if _, err := adapter.Embed(context.Background(), EmbeddingRequest{
		Model:      "gemini-embedding-001",
		Input:      []string{"یک", "دو"},
		Dimensions: &dims,
	}); err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if len(got.Requests) != 2 {
		t.Fatalf("got %d sub-requests, want 2", len(got.Requests))
	}
	// Gemini sets the width on each sub-request, not once for the batch, so
	// every entry has to carry it.
	for i, req := range got.Requests {
		if req["outputDimensionality"] != float64(1536) {
			t.Errorf("request %d: outputDimensionality = %v, want 1536", i, req["outputDimensionality"])
		}
	}
}
