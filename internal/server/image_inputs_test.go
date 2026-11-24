package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"nabugate/internal/agent"
	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

func dataURI(mediaType string, n int) string {
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(make([]byte, n))
}

func TestInputImagesParsing(t *testing.T) {
	png := dataURI("image/png", 10)
	for _, c := range []struct {
		name          string
		image, images string
		want          []string
	}{
		{"none", ``, ``, nil},
		{"null", `null`, `null`, nil},
		{"one url in images", ``, `["https://example.com/a.png"]`, []string{"https://example.com/a.png"}},
		{"single image string", `"https://example.com/a.png"`, ``, []string{"https://example.com/a.png"}},
		{"image first, then images", `"https://example.com/a.png"`, `["` + png + `"]`, []string{"https://example.com/a.png", png}},
		{"images as a string", ``, `"https://example.com/b.webp"`, []string{"https://example.com/b.webp"}},
		{"jpeg and webp data uris", ``, `["` + dataURI("image/jpeg", 3) + `","` + dataURI("image/webp", 3) + `"]`, []string{dataURI("image/jpeg", 3), dataURI("image/webp", 3)}},
		{"upper-case media type", ``, `["data:IMAGE/PNG;BASE64,AAAA"]`, []string{"data:IMAGE/PNG;BASE64,AAAA"}},
		{"trimmed", ``, `["  https://example.com/a.png  "]`, []string{"https://example.com/a.png"}},
	} {
		got, msg := inputImages(json.RawMessage(c.image), json.RawMessage(c.images))
		if msg != "" {
			t.Errorf("%s: refused: %s", c.name, msg)
			continue
		}
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestInputImagesValidation(t *testing.T) {
	fiveURLs := `["https://e.com/1.png","https://e.com/2.png","https://e.com/3.png","https://e.com/4.png","https://e.com/5.png"]`
	big := dataURI("image/png", 11<<20)
	for _, c := range []struct {
		name          string
		image, images string
		wantMsg       string
	}{
		{"too many", ``, fiveURLs, "too many input images: 5 sent, at most 4"},
		{"too many across both fields", `"https://e.com/0.png"`, `["https://e.com/1.png","https://e.com/2.png","https://e.com/3.png","https://e.com/4.png"]`, "too many input images"},
		{"plain http", ``, `["http://example.com/a.png"]`, "input image 1: must be an https:// URL"},
		{"file path", ``, `["/etc/passwd"]`, "must be an https:// URL"},
		{"no host", ``, `["https:///a.png"]`, "must be an https:// URL"},
		{"empty", ``, `[""]`, "input image 1: is empty"},
		{"gif data uri", ``, `["data:image/gif;base64,R0lGOD=="]`, `type "image/gif" is not accepted`},
		{"svg data uri", ``, `["data:image/svg+xml;base64,PHN2Zz4="]`, "is not accepted"},
		{"not base64-encoded", ``, `["data:image/png,rawbytes"]`, "must be base64-encoded"},
		{"bad base64", ``, `["data:image/png;base64,!!!notbase64"]`, "not valid base64"},
		{"empty payload", ``, `["data:image/png;base64,"]`, "payload is empty"},
		{"no comma", ``, `["data:image/png;base64"]`, "no ','"},
		{"numbers", ``, `[1,2]`, "'images' must be a string or an array of strings"},
		{"object", `{"url":"https://e.com/a.png"}`, ``, "'image' must be a string or an array of strings"},
		{"over 20 MB together", ``, `["` + big + `","` + big + `"]`, "input images are too large"},
		{"second one bad", ``, `["https://e.com/a.png","ftp://e.com/b.png"]`, "input image 2:"},
	} {
		got, msg := inputImages(json.RawMessage(c.image), json.RawMessage(c.images))
		if msg == "" {
			t.Errorf("%s: accepted %d inputs; want refusal", c.name, len(got))
			continue
		}
		if !strings.Contains(msg, c.wantMsg) {
			t.Errorf("%s: message %q does not contain %q", c.name, msg, c.wantMsg)
		}
	}
}

// editStudio is a text-only image provider that records what it was asked;
// editorStudio is the same one saying it reads input pictures.
type editStudio struct {
	name string
	got  *provider.ImageRequest
}

func (s editStudio) Name() string { return s.name }
func (s editStudio) Chat(context.Context, provider.ChatRequest) (provider.ChatResponse, error) {
	return provider.ChatResponse{}, nil
}
func (s editStudio) Image(_ context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
	if s.got != nil {
		*s.got = req
	}
	return provider.ImageResponse{Images: []string{"aGk="}}, nil
}

type editorStudio struct{ editStudio }

func (editorStudio) AcceptsInputImages(model string) bool { return model == "wan2.6-image" }

func editServer(t *testing.T, got *provider.ImageRequest) *httptest.Server {
	t.Helper()
	adapters := map[string]provider.Adapter{
		"dashscope-image": editorStudio{editStudio{name: "dashscope-image", got: got}},
		"pexels":          editStudio{name: "pexels"},
	}
	images := map[string]config.ModelRoute{
		"nabu-image": {
			Primary:  config.Target{Provider: "dashscope-image", Model: "wan2.6-t2i"},
			Fallback: []config.Target{{Provider: "pexels", Model: "search"}},
		},
		"nabu-image-edit": {Primary: config.Target{Provider: "dashscope-image", Model: "wan2.6-image"}},
	}
	r := router.New(adapters, nil, images, nil, nil, nil, nil, discardLogger())
	prices := map[string]usage.Price{
		"dashscope-image/wan2.6-t2i":   {PerImage: 0.031},
		"dashscope-image/wan2.6-image": {PerImage: 0.031},
	}
	srv := New(r, policy.New(nil, nil), usage.New(prices), agent.NewRegistry(), discardLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func postImages(t *testing.T, ts *httptest.Server, body string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Post(ts.URL+"/v1/images/generations", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// An edit reaches the editing model with its inputs and is billed per output
// picture, exactly like a generation on the same model.
func TestImageEditIsServedAndMeteredLikeAGeneration(t *testing.T) {
	var got provider.ImageRequest
	ts := editServer(t, &got)
	png := dataURI("image/png", 64)

	resp, body := postImages(t, ts, `{"model":"nabu-image-edit","prompt":"watercolor","image":"https://example.com/a.png","images":["`+png+`"]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("edit answered %d: %s", resp.StatusCode, body)
	}
	if len(got.Images) != 2 || got.Images[0] != "https://example.com/a.png" || got.Images[1] != png || got.Model != "wan2.6-image" {
		t.Fatalf("adapter got %+v", got)
	}
	cost, _ := strconv.ParseFloat(resp.Header.Get(costHeader), 64)
	if math.Abs(cost-0.031) > 1e-9 {
		t.Fatalf("edit charged %v, want 0.031 (one output picture)", cost)
	}
	var out struct {
		Data []map[string]string `json:"data"`
	}
	if json.Unmarshal([]byte(body), &out) != nil || len(out.Data) != 1 || out.Data[0]["b64_json"] != "aGk=" {
		t.Fatalf("response = %s", body)
	}

	// Without inputs the same endpoint is unchanged: no images reach the adapter.
	got = provider.ImageRequest{}
	if resp, body := postImages(t, ts, `{"model":"nabu-image","prompt":"a lemon"}`); resp.StatusCode != http.StatusOK || got.Images != nil {
		t.Fatalf("plain generation: %d %s, images %v", resp.StatusCode, body, got.Images)
	}
}

func TestImageEditRefusals(t *testing.T) {
	ts := editServer(t, nil)

	// A malformed input is the caller's to fix.
	resp, body := postImages(t, ts, `{"model":"nabu-image-edit","prompt":"x","images":["http://example.com/a.png"]}`)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "https://") {
		t.Fatalf("http input: %d %s", resp.StatusCode, body)
	}

	// An alias with nothing that edits says so instead of returning a stock
	// photo or a fresh picture.
	resp, body = postImages(t, ts, `{"model":"nabu-image","prompt":"x","images":["https://example.com/a.png"]}`)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "no model that accepts input images") {
		t.Fatalf("edit on a text-only alias: %d %s", resp.StatusCode, body)
	}

	// A body past the endpoint's cap is 413, not "invalid JSON".
	huge := `{"model":"nabu-image-edit","prompt":"x","images":["data:image/png;base64,` + strings.Repeat("A", imageBodyLimit) + `"]}`
	resp, body = postImages(t, ts, huge)
	if resp.StatusCode != http.StatusRequestEntityTooLarge || !strings.Contains(body, "too large") {
		t.Fatalf("oversized body: %d %s", resp.StatusCode, body)
	}

	// 20 MB of pictures fits the endpoint even though it is past the chat cap.
	ok := `{"model":"nabu-image-edit","prompt":"x","images":["` + dataURI("image/png", 10<<20) + `","` + dataURI("image/jpeg", 9<<20) + `"]}`
	if len(ok) <= maxRequestBytes {
		t.Fatalf("test body %d bytes does not exceed the chat cap", len(ok))
	}
	if resp, body := postImages(t, ts, ok); resp.StatusCode != http.StatusOK {
		t.Fatalf("19 MB of inputs: %d %s", resp.StatusCode, body)
	}
}

func TestModelsListsTheImageEditAlias(t *testing.T) {
	ts := editServer(t, nil)
	resp, err := http.Get(ts.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	for _, m := range out.Data {
		if m.ID == "nabu-image-edit" {
			return
		}
	}
	t.Fatalf("/v1/models = %+v; nabu-image-edit missing", out.Data)
}
