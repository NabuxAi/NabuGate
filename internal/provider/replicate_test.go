package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// replicateServer records what the adapter sent and replies with the given
// prediction bodies, one per call, so a test can script create-then-poll.
type replicateServer struct {
	*httptest.Server
	paths   []string
	bodies  []map[string]any
	prefers []string
}

func newReplicateServer(t *testing.T, replies ...string) *replicateServer {
	t.Helper()
	rs := &replicateServer{}
	call := 0
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/file.png" {
			w.Write([]byte("PNGBYTES"))
			return
		}
		rs.paths = append(rs.paths, r.URL.Path)
		rs.prefers = append(rs.prefers, r.Header.Get("Prefer"))
		if r.Method == http.MethodPost {
			var body map[string]any
			raw, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("unreadable request body: %v", err)
			}
			rs.bodies = append(rs.bodies, body)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer r8-test" {
			t.Errorf("Authorization = %q", got)
		}
		i := call
		if i >= len(replies) {
			i = len(replies) - 1
		}
		call++
		w.Header().Set("Content-Type", "application/json")
		// Scripted replies carry absolute links (a poll URL, a file URL) that
		// only exist once this server has an address, so "URL" stands in for it.
		w.Write([]byte(strings.ReplaceAll(replies[i], "URL", rs.URL)))
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *replicateServer) adapter() *ReplicateAdapter {
	return NewReplicateAdapter("replicate", rs.URL, "r8-test")
}

func (rs *replicateServer) input(t *testing.T, call int) map[string]any {
	t.Helper()
	if call >= len(rs.bodies) {
		t.Fatalf("no request body for call %d (got %d)", call, len(rs.bodies))
	}
	in, ok := rs.bodies[call]["input"].(map[string]any)
	if !ok {
		t.Fatalf("call %d has no input object: %+v", call, rs.bodies[call])
	}
	return in
}

func ptrF(v float64) *float64 { return &v }
func ptrI(v int) *int         { return &v }

// A bare "owner/name" runs the model's current version, which Replicate serves
// from its own path — not from /predictions, which requires a version id the
// caller did not give us.
func TestReplicateChatRunsAVersionlessModelOnItsOwnPath(t *testing.T) {
	rs := newReplicateServer(t, `{
		"id": "p1", "status": "succeeded",
		"output": ["Hel", "lo", " world"],
		"metrics": {"input_token_count": 11, "output_token_count": 3}
	}`)

	resp, err := rs.adapter().Chat(context.Background(), ChatRequest{
		Model: "meta/meta-llama-3-8b-instruct",
		Messages: []Message{
			{Role: "system", Content: "be brief"},
			{Role: "user", Content: "hi"},
		},
		MaxTokens:   ptrI(64),
		Temperature: ptrF(0.2),
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if rs.paths[0] != "/models/meta/meta-llama-3-8b-instruct/predictions" {
		t.Errorf("path = %q", rs.paths[0])
	}
	// A pinned version must not be invented for a model that named none.
	if _, pinned := rs.bodies[0]["version"]; pinned {
		t.Errorf("sent a version the caller never asked for: %+v", rs.bodies[0])
	}
	// Prefer: wait is what keeps the common case to a single round trip.
	if !strings.HasPrefix(rs.prefers[0], "wait=") {
		t.Errorf("Prefer = %q; without it every call would have to poll", rs.prefers[0])
	}

	in := rs.input(t, 0)
	if in["prompt"] != "hi" {
		t.Errorf("prompt = %v; a single turn should not grow role labels", in["prompt"])
	}
	if in["system_prompt"] != "be brief" {
		t.Errorf("system_prompt = %v", in["system_prompt"])
	}
	if in["max_tokens"] != float64(64) || in["temperature"] != 0.2 {
		t.Errorf("params not forwarded: %+v", in)
	}
	// top_p was not set by the caller and must not appear: a model that rejects
	// unknown or defaulted fields would fail a request nobody made.
	if _, ok := in["top_p"]; ok {
		t.Errorf("top_p invented: %+v", in)
	}

	// Text models emit token fragments. Joining them on anything but "" is how
	// you get "Hel lo world".
	if resp.Content != "Hello world" {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.Usage.PromptTokens != 11 || resp.Usage.CompletionTokens != 3 || resp.Usage.TotalTokens != 14 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

// "owner/name:version" pins the run. Only the version id identifies it, and it
// goes to the generic endpoint.
func TestReplicateChatPinsAVersion(t *testing.T) {
	rs := newReplicateServer(t, `{"id":"p2","status":"succeeded","output":"ok"}`)

	if _, err := rs.adapter().Chat(context.Background(), ChatRequest{
		Model:    "meta/meta-llama-3-8b-instruct:5a6809ca",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}); err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if rs.paths[0] != "/predictions" {
		t.Errorf("path = %q", rs.paths[0])
	}
	if rs.bodies[0]["version"] != "5a6809ca" {
		t.Errorf("version = %v", rs.bodies[0]["version"])
	}
}

// Replicate hosts thousands of models, each with its own input schema. A JSON
// prompt is the escape hatch for the ones that do not look like the common
// shape, and it has to be used verbatim — a mapped field layered on top would
// be the adapter arguing with a schema it cannot read.
func TestReplicateChatUsesAJSONPromptAsTheInputVerbatim(t *testing.T) {
	rs := newReplicateServer(t, `{"id":"p3","status":"succeeded","output":"ok"}`)

	if _, err := rs.adapter().Chat(context.Background(), ChatRequest{
		Model:     "some/odd-model",
		Messages:  []Message{{Role: "user", Content: `{"text_input":"hi","beams":4}`}},
		MaxTokens: ptrI(99),
	}); err != nil {
		t.Fatalf("Chat: %v", err)
	}

	in := rs.input(t, 0)
	if in["text_input"] != "hi" || in["beams"] != float64(4) {
		t.Errorf("input not passed through: %+v", in)
	}
	if _, ok := in["prompt"]; ok {
		t.Errorf("mapped fields layered over the caller's own schema: %+v", in)
	}
	if _, ok := in["max_tokens"]; ok {
		t.Errorf("max_tokens layered over the caller's own schema: %+v", in)
	}
}

// Prose that happens to open with a brace is prose, not a broken schema. A
// Persian sentence starting with "{" must still get an answer.
func TestReplicateChatTreatsUnparseableBracesAsProse(t *testing.T) {
	rs := newReplicateServer(t, `{"id":"p4","status":"succeeded","output":"ok"}`)

	if _, err := rs.adapter().Chat(context.Background(), ChatRequest{
		Model:    "meta/llama",
		Messages: []Message{{Role: "user", Content: "{این یک پرانتز است"}},
	}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got := rs.input(t, 0)["prompt"]; got != "{این یک پرانتز است" {
		t.Errorf("prompt = %v", got)
	}
}

// Prefer: wait covers a warm model, not a cold boot. When create comes back
// still running, the adapter has to poll rather than hand back empty output.
func TestReplicateChatPollsWhenTheCreateDidNotFinish(t *testing.T) {
	rs := newReplicateServer(t,
		`{"id":"p5","status":"processing","urls":{"get":"URL/predictions/p5"}}`,
		`{"id":"p5","status":"succeeded","output":"done"}`,
	)
	resp, err := rs.adapter().Chat(context.Background(), ChatRequest{
		Model:    "meta/llama",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "done" {
		t.Errorf("content = %q", resp.Content)
	}
	if len(rs.paths) != 2 || rs.paths[1] != "/predictions/p5" {
		t.Errorf("paths = %v; the adapter did not poll", rs.paths)
	}
}

// A failed prediction is an error, not an empty answer. The router can fall
// back from an error; it cannot fall back from a successful blank.
func TestReplicateChatFailsLoudlyOnAFailedPrediction(t *testing.T) {
	rs := newReplicateServer(t, `{"id":"p6","status":"failed","error":"CUDA out of memory"}`)

	_, err := rs.adapter().Chat(context.Background(), ChatRequest{
		Model:    "meta/llama",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("a failed prediction was reported as success")
	}
	if !strings.Contains(err.Error(), "CUDA out of memory") {
		t.Errorf("error does not carry the upstream reason: %v", err)
	}
}

// An image model behind a chat alias is an everyday config mistake. The error
// has to send whoever hits it to their config, not into this adapter.
func TestReplicateChatNamesTheShapeWhenTheOutputIsNotText(t *testing.T) {
	rs := newReplicateServer(t, `{"id":"p7","status":"succeeded","output":{"seed":12,"nsfw":false}}`)

	_, err := rs.adapter().Chat(context.Background(), ChatRequest{
		Model:    "black-forest-labs/flux-schnell",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("a non-text output was accepted as an answer")
	}
	if !strings.Contains(err.Error(), "image alias") || !strings.Contains(err.Error(), "flux-schnell") {
		t.Errorf("error does not point at the fix: %v", err)
	}
}

// Images come back as file URLs. The gateway's contract is base64, so the
// adapter downloads them — an OpenAI image client cannot follow a URL it was
// never handed.
func TestReplicateImageDownloadsAndEncodesTheFiles(t *testing.T) {
	rs := newReplicateServer(t, `{"id":"p8","status":"succeeded","output":["URL/file.png"]}`)

	resp, err := rs.adapter().Image(context.Background(), ImageRequest{
		Model:       "black-forest-labs/flux-schnell",
		Prompt:      "a cat",
		N:           1,
		AspectRatio: "16:9",
	})
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	want := base64.StdEncoding.EncodeToString([]byte("PNGBYTES"))
	if len(resp.Images) != 1 || resp.Images[0] != want {
		t.Errorf("images = %v", resp.Images)
	}

	in := rs.input(t, 0)
	if in["prompt"] != "a cat" || in["num_outputs"] != float64(1) || in["aspect_ratio"] != "16:9" {
		t.Errorf("input = %+v", in)
	}
}
