package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiChatConvertsImageURLParts(t *testing.T) {
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		if !strings.Contains(r.URL.RawQuery, "key=") {
			t.Errorf("key missing from query")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"a shoe"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":2,"totalTokenCount":5}}`))
	}))
	defer srv.Close()

	raw := `[{"role":"user","content":[{"type":"text","text":"what is this"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJD"}},{"type":"image_url","image_url":{"url":"https://cdn.example/shots/screen.jpg?x=1"}}]}]`
	resp, err := NewGeminiAdapter("gemini", srv.URL, "test-key").Chat(context.Background(), ChatRequest{
		Model: "gemini-2.5-flash",
		Raw:   map[string]json.RawMessage{"messages": json.RawMessage(raw)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "a shoe" {
		t.Fatalf("content %q", resp.Content)
	}
	var body struct {
		Contents []struct {
			Parts []map[string]any `json:"parts"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Contents) != 1 || len(body.Contents[0].Parts) != 3 {
		t.Fatalf("parts = %#v", body.Contents)
	}
	parts := body.Contents[0].Parts
	if parts[0]["text"] != "what is this" {
		t.Fatalf("text part = %#v", parts[0])
	}
	inline, _ := parts[1]["inlineData"].(map[string]any)
	if inline["mimeType"] != "image/png" || inline["data"] != "QUJD" {
		t.Fatalf("inlineData = %#v", parts[1])
	}
	file, _ := parts[2]["fileData"].(map[string]any)
	if file["fileUri"] != "https://cdn.example/shots/screen.jpg?x=1" || file["mimeType"] != "image/jpeg" {
		t.Fatalf("fileData = %#v", parts[2])
	}
}

func TestGeminiChatRejectsCleartextImageURLs(t *testing.T) {
	raw := `[{"role":"user","content":[{"type":"image_url","image_url":{"url":"http://cdn.example/a.png"}}]}]`
	_, _, err := buildGeminiContents(ChatRequest{Raw: map[string]json.RawMessage{"messages": json.RawMessage(raw)}})
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("err = %v", err)
	}
}
