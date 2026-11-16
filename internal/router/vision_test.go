package router

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// nabu-vision's Gemini rung is Google's OpenAI-compatible endpoint, so an
// image_url part is forwarded as the caller wrote it: a data URL and an
// https URL both have to arrive unchanged when the GPT-4o rung fails.
func TestNabuVisionFallsBackToGeminiWithImageURLs(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"down"}}`))
	}))
	defer primary.Close()

	var got []byte
	var model string
	gemini := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		var body struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(got, &body)
		model = body.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"a receipt"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`))
	}))
	defer gemini.Close()

	r := New(map[string]provider.Adapter{
		"openai":        provider.NewOpenAIAdapter("openai", primary.URL, "k", nil),
		"gemini-openai": provider.NewOpenAIAdapter("gemini-openai", gemini.URL, "k", nil),
	}, map[string]config.ModelRoute{
		"nabu-vision": {
			Primary: config.Target{Provider: "openai", Model: "gpt-4o"},
			Fallback: []config.Target{{
				Provider: "gemini-openai",
				Model:    "gemini-2.5-flash",
			}},
		},
	}, nil, nil, nil, nil, nil, discardLogger())

	messages := `[{"role":"user","content":[{"type":"text","text":"read this"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJD"}},{"type":"image_url","image_url":{"url":"https://cdn.example/shot.png"}}]}]`
	_, err := r.Chat(context.Background(), "nabu-vision", provider.ChatRequest{
		Raw: map[string]json.RawMessage{"messages": json.RawMessage(messages)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if model != "gemini-2.5-flash" {
		t.Fatalf("model = %q", model)
	}
	for _, want := range []string{
		`"text":"read this"`,
		`data:image/png;base64,QUJD`,
		`https://cdn.example/shot.png`,
		`"type":"image_url"`,
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("gemini body missing %s\n%s", want, got)
		}
	}
}
