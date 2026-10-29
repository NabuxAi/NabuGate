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
