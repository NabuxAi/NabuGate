package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDashScopeImageGeneratesAndDownloads(t *testing.T) {
	imgBytes := []byte("\x89PNG-fake-bytes")
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/img/1.png":
			_, _ = w.Write(imgBytes)
		case "/services/aigc/multimodal-generation/generation":
			if r.Header.Get("Authorization") != "Bearer k" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":"InvalidApiKey","message":"bad key"}`))
				return
			}
			_ = json.NewDecoder(r.Body).Decode(&got)
			img := "http://" + r.Host + "/img/1.png"
			_, _ = w.Write([]byte(`{"output":{"choices":[{"message":{"content":[{"image":"` + img + `"}]}}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	a := NewDashScopeImageAdapter("dashscope-image", srv.URL, "k")
	resp, err := a.Image(context.Background(), ImageRequest{Model: "wan2.6-t2i", Prompt: "a lighthouse", AspectRatio: "16:9", NegativePrompt: "text"})
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	if len(resp.Images) != 1 || resp.Images[0] != base64.StdEncoding.EncodeToString(imgBytes) {
		t.Fatalf("images = %v", resp.Images)
	}
	params, _ := got["parameters"].(map[string]any)
	if got["model"] != "wan2.6-t2i" || params["size"] != "1696*960" || params["n"] != float64(1) || params["negative_prompt"] != "text" {
		t.Fatalf("request body = %v", got)
	}

	if _, err := NewDashScopeImageAdapter("d", srv.URL, "wrong").Image(context.Background(), ImageRequest{Model: "wan2.6-t2i", Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("want the upstream message, got %v", err)
	}
}

// With input pictures the user turn is one {"image"} part per picture and then
// the prompt, and size is left out unless asked for so the edit keeps the
// input's shape. This is the body verified live against wan2.6-image.
func TestDashScopeImageEditBody(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/img/1.png":
			_, _ = w.Write([]byte("edited"))
		case "/services/aigc/multimodal-generation/generation":
			_ = json.NewDecoder(r.Body).Decode(&got)
			_, _ = w.Write([]byte(`{"output":{"choices":[{"message":{"content":[{"image":"http://` + r.Host + `/img/1.png"}]}}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	a := NewDashScopeImageAdapter("dashscope-image", srv.URL, "k")
	inputs := []string{"https://example.com/photo.png", "data:image/png;base64,iVBORw0KGgo="}
	resp, err := a.Image(context.Background(), ImageRequest{Model: "wan2.6-image", Prompt: "Make it look like a watercolor painting.", Images: inputs})
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	if len(resp.Images) != 1 || resp.Images[0] != base64.StdEncoding.EncodeToString([]byte("edited")) {
		t.Fatalf("images = %v", resp.Images)
	}
	want := `{"input":{"messages":[{"content":[{"image":"https://example.com/photo.png"},{"image":"data:image/png;base64,iVBORw0KGgo="},{"text":"Make it look like a watercolor painting."}],"role":"user"}]},"model":"wan2.6-image","parameters":{"n":1,"watermark":false}}`
	if b, _ := json.Marshal(got); string(b) != want {
		t.Fatalf("edit body =\n%s\nwant\n%s", b, want)
	}

	// An explicit shape is still honoured on an edit.
	body := dashScopeImageBody(ImageRequest{Model: "wan2.6-image", Prompt: "p", Images: inputs[:1], AspectRatio: "9:16"}, 1)
	if size := body["parameters"].(map[string]any)["size"]; size != "960*1696" {
		t.Fatalf("edit with aspect ratio: size = %v", size)
	}
}

// Without input pictures the body is the text-to-image one, byte for byte:
// one text part, and a size even when none was asked for.
func TestDashScopeImageBodyWithoutInputsIsUnchanged(t *testing.T) {
	body := dashScopeImageBody(ImageRequest{Model: "wan2.6-t2i", Prompt: "a lighthouse"}, 1)
	want := `{"input":{"messages":[{"content":[{"text":"a lighthouse"}],"role":"user"}]},"model":"wan2.6-t2i","parameters":{"n":1,"size":"1280*1280","watermark":false}}`
	if b, _ := json.Marshal(body); string(b) != want {
		t.Fatalf("text-to-image body =\n%s\nwant\n%s", b, want)
	}
}

func TestDashScopeAcceptsInputImagesPerModel(t *testing.T) {
	a := NewDashScopeImageAdapter("dashscope-image", "", "k")
	var _ ImageInputAdapter = a
	for model, want := range map[string]bool{"wan2.6-image": true, "wan2.6-t2i": false, "WAN2.5-T2I-PREVIEW": false} {
		if got := a.AcceptsInputImages(model); got != want {
			t.Errorf("AcceptsInputImages(%q) = %v, want %v", model, got, want)
		}
	}
}

func TestDashScopeSize(t *testing.T) {
	for _, c := range []struct {
		req  ImageRequest
		want string
	}{
		{ImageRequest{AspectRatio: "9:16"}, "960*1696"},
		{ImageRequest{Size: "1536x1024"}, "1536*1024"},
		{ImageRequest{}, "1280*1280"},
	} {
		if got := dashScopeSize(c.req); got != c.want {
			t.Errorf("dashScopeSize(%+v) = %q, want %q", c.req, got, c.want)
		}
	}
}

// Live: DASHSCOPE_API_KEY=... go test -run TestDashScopeImageLive ./internal/provider/
func TestDashScopeImageLive(t *testing.T) {
	key := os.Getenv("DASHSCOPE_API_KEY")
	if key == "" {
		t.Skip("DASHSCOPE_API_KEY not set")
	}
	resp, err := NewDashScopeImageAdapter("dashscope-image", "", key).Image(context.Background(),
		ImageRequest{Model: "wan2.6-t2i", Prompt: "a red apple on a wooden desk", AspectRatio: "16:9"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(resp.Images[0])
	if !strings.HasPrefix(string(raw), "\x89PNG") && !strings.HasPrefix(string(raw), "\xff\xd8") {
		t.Fatalf("not an image: %d bytes", len(raw))
	}
}
