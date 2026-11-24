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

// openRouterImageAnswer is the shape OpenRouter answers an image-output model
// with on /chat/completions: the picture as a data: URI under
// message.images, a line of text beside it, and usage carrying the cost in
// USD when the request asked for usage.include. Recorded from OpenRouter's
// Gemini image route; the base64 here is a stand-in.
const openRouterImageAnswer = `{
  "id": "gen-1764000000-AbCdEfGhIjKlMnOp",
  "provider": "Google AI Studio",
  "model": "google/gemini-2.5-flash-image",
  "object": "chat.completion",
  "created": 1764000000,
  "choices": [{
    "logprobs": null,
    "finish_reason": "stop",
    "native_finish_reason": "STOP",
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "Here is a lighthouse at dusk.",
      "refusal": null,
      "reasoning": null,
      "images": [{"type": "image_url", "image_url": {"url": "data:image/png;base64,UE5HLWJ5dGVz"}, "index": 0}]
    }
  }],
  "usage": {
    "prompt_tokens": 12,
    "completion_tokens": 1300,
    "total_tokens": 1312,
    "cost": 0.0387243,
    "is_byok": false,
    "prompt_tokens_details": {"cached_tokens": 0},
    "cost_details": {"upstream_inference_cost": null},
    "completion_tokens_details": {"reasoning_tokens": 0, "image_tokens": 1290}
  }
}`

func chatImageServer(t *testing.T, answer string, status int, got *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/files/pic.png":
			_, _ = w.Write([]byte("downloaded-bytes"))
		case "/chat/completions":
			if r.Header.Get("Authorization") != "Bearer k" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			raw, _ := io.ReadAll(r.Body)
			if got != nil {
				_ = json.Unmarshal(raw, got)
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(strings.ReplaceAll(answer, "HOST", r.Host)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func chatImageAdapter(t *testing.T, baseURL string) *OpenAIAdapter {
	t.Helper()
	a := NewOpenAIAdapter("openrouter", baseURL, "k", nil)
	if err := a.SetImageAPI("chat"); err != nil {
		t.Fatal(err)
	}
	return a
}

// The request is one user turn — the prompt, then each input picture as an
// image_url part — with modalities ["image","text"] and usage.include, and
// an aspect ratio only when one was asked for.
func TestChatImageRequestMapping(t *testing.T) {
	for _, c := range []struct {
		name string
		req  ImageRequest
		want string
	}{
		{"text to image, no shape",
			ImageRequest{Model: "google/gemini-3.1-flash-image", Prompt: "a lighthouse"},
			`{"messages":[{"content":[{"text":"a lighthouse","type":"text"}],"role":"user"}],"modalities":["image","text"],"model":"google/gemini-3.1-flash-image","usage":{"include":true}}`},
		{"aspect ratio passes through",
			ImageRequest{Model: "google/gemini-3.1-flash-image", Prompt: "p", AspectRatio: "9:16"},
			`{"image_config":{"aspect_ratio":"9:16"},"messages":[{"content":[{"text":"p","type":"text"}],"role":"user"}],"modalities":["image","text"],"model":"google/gemini-3.1-flash-image","usage":{"include":true}}`},
		{"a WxH size becomes its nearest ratio",
			ImageRequest{Model: "openai/gpt-5-image", Prompt: "p", Size: "1536x1024"},
			`{"image_config":{"aspect_ratio":"3:2"},"messages":[{"content":[{"text":"p","type":"text"}],"role":"user"}],"modalities":["image","text"],"model":"openai/gpt-5-image","usage":{"include":true}}`},
		{"an edit carries its pictures as image_url parts",
			ImageRequest{Model: "google/gemini-2.5-flash-image", Prompt: "make it a watercolor", Images: []string{"https://example.com/a.png", "data:image/png;base64,iVBORw0KGgo="}},
			`{"messages":[{"content":[{"text":"make it a watercolor","type":"text"},{"image_url":{"url":"https://example.com/a.png"},"type":"image_url"},{"image_url":{"url":"data:image/png;base64,iVBORw0KGgo="},"type":"image_url"}],"role":"user"}],"modalities":["image","text"],"model":"google/gemini-2.5-flash-image","usage":{"include":true}}`},
	} {
		got, _ := json.Marshal(chatImageBody(c.req))
		if string(got) != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

// The recorded answer is parsed into the picture's bytes, the tokens and the
// reported cost.
func TestChatImageParsesTheRecordedAnswer(t *testing.T) {
	var sent map[string]any
	srv := chatImageServer(t, openRouterImageAnswer, http.StatusOK, &sent)
	a := chatImageAdapter(t, srv.URL)
	var _ ImageInputAdapter = a

	resp, err := a.Image(context.Background(), ImageRequest{Model: "google/gemini-2.5-flash-image", Prompt: "a lighthouse at dusk", AspectRatio: "16:9"})
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	if raw, _ := base64.StdEncoding.DecodeString(firstOr(resp.Images)); len(resp.Images) != 1 || string(raw) != "PNG-bytes" {
		t.Fatalf("images = %v", resp.Images)
	}
	if !resp.CostReported || resp.CostUSD != 0.0387243 {
		t.Fatalf("cost = %v (reported %v)", resp.CostUSD, resp.CostReported)
	}
	if resp.Usage.PromptTokens != 12 || resp.Usage.CompletionTokens != 1300 || resp.Usage.TotalTokens != 1312 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	if sent["model"] != "google/gemini-2.5-flash-image" || sent["image_config"].(map[string]any)["aspect_ratio"] != "16:9" {
		t.Fatalf("sent %v", sent)
	}
}

func firstOr(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

func TestChatImageOtherAnswers(t *testing.T) {
	// A URL instead of a data: URI is downloaded; no cost reported is said so;
	// n caps what is returned.
	answer := `{"choices":[{"message":{"role":"assistant","content":null,"images":[` +
		`{"type":"image_url","image_url":{"url":"http://HOST/files/pic.png"}},` +
		`{"type":"image_url","image_url":{"url":"data:image/webp;base64,c2Vjb25k"}}]}}],` +
		`"usage":{"prompt_tokens":5,"completion_tokens":4000,"total_tokens":4005}}`
	a := chatImageAdapter(t, chatImageServer(t, answer, http.StatusOK, nil).URL)
	resp, err := a.Image(context.Background(), ImageRequest{Model: "openai/gpt-5-image-mini", Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Images) != 1 || resp.Images[0] != base64.StdEncoding.EncodeToString([]byte("downloaded-bytes")) || resp.CostReported {
		t.Fatalf("resp = %+v", resp)
	}
	if resp, err = a.Image(context.Background(), ImageRequest{Model: "openai/gpt-5-image-mini", Prompt: "p", N: 2}); err != nil || len(resp.Images) != 2 || resp.Images[1] != "c2Vjb25k" {
		t.Fatalf("n=2: %+v %v", resp, err)
	}

	// A model that answers with words only is not an image.
	a = chatImageAdapter(t, chatImageServer(t, `{"choices":[{"message":{"content":"I can only describe pictures."}}],"usage":{"cost":0.0001}}`, http.StatusOK, nil).URL)
	if _, err := a.Image(context.Background(), ImageRequest{Model: "deepseek/deepseek-v4.1-flash", Prompt: "p"}); err == nil || !strings.Contains(err.Error(), "text and no image") {
		t.Fatalf("text-only answer: %v", err)
	}

	// The account with no credit: the status reaches the error, which is
	// what marks the provider out of credit.
	a = chatImageAdapter(t, chatImageServer(t, `{"error":{"message":"This request requires more credits, or fewer max_tokens. You have never purchased credits.","code":402}}`, http.StatusPaymentRequired, nil).URL)
	if _, err := a.Image(context.Background(), ImageRequest{Model: "google/gemini-3-pro-image", Prompt: "p"}); err == nil ||
		!strings.Contains(err.Error(), "status 402") || !strings.Contains(err.Error(), "never purchased credits") {
		t.Fatalf("402: %v", err)
	}
}

// Only the chat route takes input pictures; OpenAI's images API is unchanged.
func TestImageAPISetting(t *testing.T) {
	a := NewOpenAIAdapter("openai", "https://example.com/v1", "k", nil)
	if a.AcceptsInputImages("gpt-image-1") {
		t.Fatal("the images API takes no input pictures")
	}
	if err := a.SetImageAPI("Chat"); err != nil || !a.AcceptsInputImages("google/gemini-3.1-flash-image") {
		t.Fatalf("chat: %v", err)
	}
	if err := a.SetImageAPI("images"); err != nil || a.AcceptsInputImages("x") {
		t.Fatalf("images: %v", err)
	}
	if err := a.SetImageAPI("responses"); err == nil {
		t.Fatal("an unknown image_api must be refused")
	}
}
