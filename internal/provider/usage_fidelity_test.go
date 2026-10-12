package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ─────────────────────────────── usage fidelity ─────────────────────────────
//
// An upstream that answers with content but no usage object used to meter at
// an indifferent zero on the non-streaming paths (OpenAI's streaming path was
// the only one that guessed). Every adapter now takes the same four-bytes-
// per-token fallback, flags the result as a guess, and leaves a vendor that
// does report usage completely untouched.

func chatRequest() ChatRequest {
	return ChatRequest{Model: "m", Messages: []Message{
		{Role: "system", Content: "you are a test"},
		{Role: "user", Content: "hello there, adapter under test"},
	}}
}

// TestOpenAIChatWithoutUsageIsEstimated: the body carries choices but no
// usage, so the counts are the gateway's guess — positive and flagged.
func TestOpenAIChatWithoutUsageIsEstimated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"an answer of some length here"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	a := NewOpenAIAdapter("openai", srv.URL, "k", nil)
	resp, err := a.Chat(context.Background(), chatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.PromptTokens <= 0 || resp.Usage.CompletionTokens <= 0 || resp.Usage.TotalTokens <= 0 {
		t.Fatalf("estimated usage = %+v, want positive counts", resp.Usage)
	}
	if !resp.Usage.Estimated {
		t.Fatal("usage is a guess but carries no Estimated flag")
	}
}

// A vendor that reports real usage is never touched by the fallback.
func TestOpenAIChatWithRealUsageIsUntouched(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`))
	}))
	defer srv.Close()

	a := NewOpenAIAdapter("openai", srv.URL, "k", nil)
	resp, err := a.Chat(context.Background(), chatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.PromptTokens != 11 || resp.Usage.CompletionTokens != 7 || resp.Usage.TotalTokens != 18 {
		t.Fatalf("usage = %+v, want the vendor's own numbers", resp.Usage)
	}
	if resp.Usage.Estimated {
		t.Fatal("a vendor's count must not be flagged as a guess")
	}
}

// TestAnthropicChatWithoutUsageIsEstimated — same fallback on the Anthropic
// shape, plus the cache-token directions parsed from a real usage object.
func TestAnthropicChatWithoutUsageIsEstimated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":[{"text":"hello from claude"}],"stop_reason":"end_turn"}`))
	}))
	defer srv.Close()

	a := NewAnthropicAdapter("anthropic", srv.URL, "k")
	resp, err := a.Chat(context.Background(), chatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.PromptTokens <= 0 || resp.Usage.CompletionTokens <= 0 {
		t.Fatalf("estimated usage = %+v, want positive counts", resp.Usage)
	}
	if !resp.Usage.Estimated {
		t.Fatal("usage is a guess but carries no Estimated flag")
	}
}

// TestAnthropicChatParsesCacheTokens: cache_creation_input_tokens and
// cache_read_input_tokens are billed at their own rates on the real invoice,
// so they are carried out of the response instead of dropped — and never
// folded into the plain input count.
func TestAnthropicChatParsesCacheTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":[{"text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":4,"cache_read_input_tokens":200,"cache_creation_input_tokens":50}}`))
	}))
	defer srv.Close()

	a := NewAnthropicAdapter("anthropic", srv.URL, "k")
	resp, err := a.Chat(context.Background(), chatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.PromptTokens != 10 || resp.Usage.CompletionTokens != 4 || resp.Usage.TotalTokens != 14 {
		t.Fatalf("usage = %+v, want 10/4/14 with cache kept out of the plain counts", resp.Usage)
	}
	if resp.Usage.CacheReadInput != 200 || resp.Usage.CacheCreationInput != 50 {
		t.Fatalf("cache tokens = %d read / %d creation, want 200/50", resp.Usage.CacheReadInput, resp.Usage.CacheCreationInput)
	}
	if resp.Usage.Estimated {
		t.Fatal("a vendor's count must not be flagged as a guess")
	}
}

func sse(w http.ResponseWriter, events ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, e := range events {
		fmt.Fprintf(w, "data: %s\n\n", e)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// TestAnthropicChatStreamWithoutUsageIsEstimated: the stream carries deltas
// but no message_start/message_delta usage, so the byte-based guess applies.
func TestAnthropicChatStreamWithoutUsageIsEstimated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w,
			`{"type":"content_block_delta","delta":{"text":"hello "}}`,
			`{"type":"content_block_delta","delta":{"text":"world"}}`,
		)
	}))
	defer srv.Close()

	a := NewAnthropicAdapter("anthropic", srv.URL, "k")
	var got strings.Builder
	usage, err := a.ChatStream(context.Background(), chatRequest(), func(d string) error { got.WriteString(d); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "hello world" {
		t.Fatalf("stream content = %q", got.String())
	}
	if usage.PromptTokens <= 0 || usage.CompletionTokens <= 0 {
		t.Fatalf("estimated stream usage = %+v, want positive counts", usage)
	}
	if !usage.Estimated {
		t.Fatal("stream usage is a guess but carries no Estimated flag")
	}
}

// A real Anthropic stream keeps its real numbers — message_start for the
// input side, message_delta for the output, cache directions included.
func TestAnthropicChatStreamWithRealUsageIsUntouched(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w,
			`{"type":"message_start","message":{"usage":{"input_tokens":33,"cache_read_input_tokens":7,"cache_creation_input_tokens":2}}}`,
			`{"type":"content_block_delta","delta":{"text":"answer"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":9}}`,
		)
	}))
	defer srv.Close()

	a := NewAnthropicAdapter("anthropic", srv.URL, "k")
	usage, err := a.ChatStream(context.Background(), chatRequest(), func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if usage.PromptTokens != 33 || usage.CompletionTokens != 9 || usage.TotalTokens != 42 {
		t.Fatalf("usage = %+v, want 33/9/42", usage)
	}
	if usage.CacheReadInput != 7 || usage.CacheCreationInput != 2 {
		t.Fatalf("cache tokens = %d/%d, want 7/2", usage.CacheReadInput, usage.CacheCreationInput)
	}
	if usage.Estimated {
		t.Fatal("a vendor's count must not be flagged as a guess")
	}
}

// TestGeminiChatWithoutUsageIsEstimated and its streaming twin: Gemini's
// usageMetadata can be absent on either path; the guess now covers both.
func TestGeminiChatWithoutUsageIsEstimated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"gemini says hi"}]},"finishReason":"STOP"}]}`))
	}))
	defer srv.Close()

	a := NewGeminiAdapter("gemini", srv.URL, "k")
	resp, err := a.Chat(context.Background(), chatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.PromptTokens <= 0 || resp.Usage.CompletionTokens <= 0 {
		t.Fatalf("estimated usage = %+v, want positive counts", resp.Usage)
	}
	if !resp.Usage.Estimated {
		t.Fatal("usage is a guess but carries no Estimated flag")
	}
}

func TestGeminiChatStreamWithoutUsageIsEstimated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w,
			`{"candidates":[{"content":{"parts":[{"text":"gemin"}]}}]}`,
			`{"candidates":[{"content":{"parts":[{"text":"i streams"}]}}]}`,
		)
	}))
	defer srv.Close()

	a := NewGeminiAdapter("gemini", srv.URL, "k")
	usage, err := a.ChatStream(context.Background(), chatRequest(), func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if usage.PromptTokens <= 0 || usage.CompletionTokens <= 0 {
		t.Fatalf("estimated stream usage = %+v, want positive counts", usage)
	}
	if !usage.Estimated {
		t.Fatal("stream usage is a guess but carries no Estimated flag")
	}
}

// The decision-chat route (TypeSafe/v1m riding /chat/completions) guessed only
// from the user's prompt; the estimate now covers the payload actually sent —
// the marshalled state and questions — and carries the Estimated flag.
func TestDecisionChatEstimateCoversTheWholePayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"model":"jev-latest","answers":{"intent":{"type":"choice","choice":"general_chat"}}}`))
	}))
	defer srv.Close()

	a := NewTypeSafeAdapter("typesafe", srv.URL, "k")
	req := ChatRequest{Model: "typesafe/jev-latest", Messages: []Message{{Role: "user", Content: "a short question"}}}
	resp, err := a.Chat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Usage.Estimated {
		t.Fatal("usage is a guess but carries no Estimated flag")
	}
	// The state the adapter marshals wraps the messages; the questions are the
	// whole default System One set. Together they must outweigh the bare
	// prompt by a wide margin — guessing from the prompt alone was the
	// under-count this replaces.
	if resp.Usage.PromptTokens <= EstimateTokens(len("a short question")) {
		t.Fatalf("prompt estimate %d covers nothing beyond the bare prompt", resp.Usage.PromptTokens)
	}
}

// ─────────────────────────── transcribe duration ────────────────────────────

// The transcription bill used to be built only from the end of the last word.
// A transcript with text but no word boundaries metered zero minutes of audio
// that was really uploaded — so the vendor's own audio_duration_secs, the
// field the ElevenLabs speech-to-text response documents, now fills in where
// the words end. (There is no other fallback: the adapter builds its segments
// from the words, so empty words means no segments either.)
func TestElevenLabsTranscribeDurationFallsBackToTheVendorField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"language_code":"en","text":"a wordless utterance","audio_duration_secs":12.5,"words":[]}`))
	}))
	defer srv.Close()

	a := NewElevenLabsAdapter("elevenlabs", srv.URL, "k")
	out, err := a.Transcribe(context.Background(), TranscriptionRequest{Model: "scribe_v2", Audio: []byte("OggS")})
	if err != nil {
		t.Fatal(err)
	}
	if out.Duration != 12.5 {
		t.Errorf("duration = %v, want the vendor's 12.5s", out.Duration)
	}

	// Words still win: the last word's end is the primary source, unchanged.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"text":"hello there","audio_duration_secs":30,"words":[
			{"text":"hello","start":0,"end":0.5,"type":"word"},
			{"text":"there","start":0.6,"end":1.25,"type":"word"}]}`))
	}))
	defer srv2.Close()

	a2 := NewElevenLabsAdapter("elevenlabs", srv2.URL, "k")
	out2, err := a2.Transcribe(context.Background(), TranscriptionRequest{Model: "scribe_v2", Audio: []byte("OggS")})
	if err != nil {
		t.Fatal(err)
	}
	if out2.Duration != 1.25 {
		t.Errorf("duration = %v, want the last word's 1.25s end", out2.Duration)
	}
}

// Some OpenAI-compatible servers report the prompt/completion split but omit
// total_tokens. The real split is never replaced by the byte-based guess —
// only the missing sum is completed from it.
func TestOpenAIChatSplitWithoutTotalIsKept(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`))
	}))
	defer srv.Close()

	a := NewOpenAIAdapter("openai", srv.URL, "k", nil)
	resp, err := a.Chat(context.Background(), chatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.PromptTokens != 11 || resp.Usage.CompletionTokens != 7 || resp.Usage.TotalTokens != 18 {
		t.Fatalf("usage = %+v, want the vendor's own 11/7 completed to 18", resp.Usage)
	}
	if resp.Usage.Estimated {
		t.Fatal("a vendor's count must not be flagged as a guess")
	}
}

func TestGeminiChatSplitWithoutTotalIsKept(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":4}}`))
	}))
	defer srv.Close()

	a := NewGeminiAdapter("gemini", srv.URL, "k")
	resp, err := a.Chat(context.Background(), chatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.PromptTokens != 9 || resp.Usage.CompletionTokens != 4 || resp.Usage.TotalTokens != 13 {
		t.Fatalf("usage = %+v, want the vendor's own 9/4 completed to 13", resp.Usage)
	}
	if resp.Usage.Estimated {
		t.Fatal("a vendor's count must not be flagged as a guess")
	}
}
