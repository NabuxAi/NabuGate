package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakePexels serves /search and /curated plus an /img/ path that returns fixed
// bytes, so the adapter can both search and download in one test server.
func fakePexels(t *testing.T, imgBytes []byte) (*httptest.Server, *string) {
	t.Helper()
	var lastQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/img/"):
			_, _ = w.Write(imgBytes)
			return
		case r.URL.Path == "/search" || r.URL.Path == "/curated":
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"no key"}`))
				return
			}
			lastQuery = r.URL.RawQuery
			base := "http://" + r.Host
			mk := func(id int) map[string]any {
				return map[string]any{
					"id": id, "url": "https://pexels.com/p/" + string(rune(id)),
					"photographer": "Jane",
					"src":          map[string]string{"large2x": base + "/img/" + string(rune('0'+id)) + ".jpg"},
				}
			}
			resp := map[string]any{"photos": []map[string]any{mk(1), mk(2), mk(3)}}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &lastQuery
}

func TestPexelsImageSearchAndDownload(t *testing.T) {
	imgBytes := []byte("\xff\xd8\xff\xe0JFIF-fake-jpeg-bytes")
	srv, lastQuery := fakePexels(t, imgBytes)
	a := NewPexelsAdapter("pexels", srv.URL, "test-key")

	resp, err := a.Image(context.Background(), ImageRequest{Prompt: "coffee cup", N: 2, AspectRatio: "16:9"})
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	if len(resp.Images) != 2 {
		t.Fatalf("got %d images, want 2", len(resp.Images))
	}
	want := base64.StdEncoding.EncodeToString(imgBytes)
	for i, img := range resp.Images {
		if img != want {
			t.Errorf("image[%d] not the downloaded bytes", i)
		}
	}
	if !strings.Contains(*lastQuery, "query=coffee+cup") {
		t.Errorf("query not forwarded: %s", *lastQuery)
	}
	if !strings.Contains(*lastQuery, "orientation=landscape") {
		t.Errorf("16:9 should map to landscape orientation: %s", *lastQuery)
	}
}

func TestPexelsCuratedWhenNoPrompt(t *testing.T) {
	srv, _ := fakePexels(t, []byte("img"))
	a := NewPexelsAdapter("pexels", srv.URL, "k")
	resp, err := a.Image(context.Background(), ImageRequest{}) // no prompt
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	if len(resp.Images) != 1 {
		t.Errorf("default N=1, got %d images", len(resp.Images))
	}
}

func TestPexelsOrientationMapping(t *testing.T) {
	cases := []struct {
		req  ImageRequest
		want string
	}{
		{ImageRequest{AspectRatio: "16:9"}, "landscape"},
		{ImageRequest{AspectRatio: "9:16"}, "portrait"},
		{ImageRequest{AspectRatio: "1:1"}, "square"},
		{ImageRequest{Size: "1792x1024"}, "landscape"},
		{ImageRequest{Size: "1024x1792"}, "portrait"},
		{ImageRequest{}, ""},
		{ImageRequest{Size: "weird"}, ""},
	}
	for _, c := range cases {
		if got := pexelsOrientation(c.req); got != c.want {
			t.Errorf("pexelsOrientation(%+v) = %q, want %q", c.req, got, c.want)
		}
	}
}

func TestPexelsNoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"photos":[]}`))
	}))
	defer srv.Close()
	a := NewPexelsAdapter("pexels", srv.URL, "k")
	_, err := a.Image(context.Background(), ImageRequest{Prompt: "nothing"})
	if err == nil {
		t.Fatal("expected an error when no photos are returned")
	}
	if !strings.Contains(err.Error(), "no photos") {
		t.Errorf("error = %v, want a 'no photos' message", err)
	}
}

// PexelsAdapter must satisfy the ImageAdapter interface.
var _ ImageAdapter = (*PexelsAdapter)(nil)
