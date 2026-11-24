package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
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
	for model, want := range map[string]bool{
		"wan2.6-image": true, "wan2.6-t2i": false, "WAN2.5-T2I-PREVIEW": false,
		"wan2.5-t2i-preview": false, "wan2.2-t2i-plus": false, "wan2.2-t2i-flash": false,
	} {
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

// fastPoll makes the async path poll at once in tests.
func fastPoll(a *DashScopeImageAdapter, timeout time.Duration) *DashScopeImageAdapter {
	a.pollEvery, a.pollMax, a.pollTimeout = time.Millisecond, 2*time.Millisecond, timeout
	return a
}

// text2imageVendor is the asynchronous text2image API: a create answers a task
// id, and /tasks/{id} walks through the answers given, then stays on the last.
type text2imageVendor struct {
	mu        sync.Mutex
	created   []map[string]any
	headers   http.Header
	polls     int
	answers   []string // task bodies; "503" answers a transient failure
	createErr string
	multi     bool // the multimodal endpoint was called
}

func (v *text2imageVendor) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.mu.Lock()
		defer v.mu.Unlock()
		switch {
		case r.URL.Path == "/img/1.png":
			_, _ = w.Write([]byte("picture-one"))
		case r.URL.Path == "/img/2.png":
			_, _ = w.Write([]byte("picture-two"))
		case r.URL.Path == "/services/aigc/multimodal-generation/generation":
			v.multi = true
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":"InvalidParameter","message":"url error"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/services/aigc/text2image/image-synthesis":
			v.headers = r.Header.Clone()
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			v.created = append(v.created, body)
			if v.createErr != "" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(v.createErr))
				return
			}
			_, _ = w.Write([]byte(`{"output":{"task_id":"t-1","task_status":"PENDING"},"request_id":"r"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/tasks/t-1":
			i := min(v.polls, len(v.answers)-1)
			v.polls++
			if v.answers[i] == "503" {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(strings.ReplaceAll(v.answers[i], "HOST", r.Host)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// wan2.2-t2i-flash and its siblings are created as a task on the text2image
// API, polled through PENDING, RUNNING and a passing 503 to SUCCEEDED, and the
// pictures that succeeded are downloaded; a result that failed on its own is
// skipped.
func TestDashScopeText2ImageCreatesPollsAndDownloads(t *testing.T) {
	v := &text2imageVendor{answers: []string{
		`{"output":{"task_id":"t-1","task_status":"PENDING"}}`,
		`{"output":{"task_id":"t-1","task_status":"RUNNING"}}`,
		"503",
		`{"output":{"task_id":"t-1","task_status":"SUCCEEDED","results":[{"code":"DataInspectionFailed","message":"unsafe"},{"url":"http://HOST/img/1.png"},{"url":"http://HOST/img/2.png"}]},"usage":{"image_count":2}}`,
	}}
	srv := v.server(t)
	a := fastPoll(NewDashScopeImageAdapter("dashscope-image", srv.URL, "k"), 5*time.Second)
	resp, err := a.Image(context.Background(), ImageRequest{Model: "wan2.2-t2i-flash", Prompt: "a lighthouse", NegativePrompt: "text", AspectRatio: "16:9", N: 2})
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	want := []string{base64.StdEncoding.EncodeToString([]byte("picture-one")), base64.StdEncoding.EncodeToString([]byte("picture-two"))}
	if strings.Join(resp.Images, ",") != strings.Join(want, ",") {
		t.Fatalf("images = %v", resp.Images)
	}
	if v.multi || len(v.created) != 1 || v.polls != 4 {
		t.Fatalf("multimodal called %v, creates %d, polls %d", v.multi, len(v.created), v.polls)
	}
	if v.headers.Get("X-DashScope-Async") != "enable" || v.headers.Get("Authorization") != "Bearer k" {
		t.Fatalf("create headers = %v", v.headers)
	}
	got, _ := json.Marshal(v.created[0])
	if wantBody := `{"input":{"negative_prompt":"text","prompt":"a lighthouse"},"model":"wan2.2-t2i-flash","parameters":{"n":2,"size":"1360*768"}}`; string(got) != wantBody {
		t.Fatalf("create body =\n%s\nwant\n%s", got, wantBody)
	}
}

func TestDashScopeText2ImageFailures(t *testing.T) {
	// A task that fails says why.
	v := &text2imageVendor{answers: []string{`{"output":{"task_id":"t-1","task_status":"FAILED","code":"DataInspectionFailed","message":"unsafe prompt"}}`}}
	a := fastPoll(NewDashScopeImageAdapter("d", v.server(t).URL, "k"), 5*time.Second)
	if _, err := a.Image(context.Background(), ImageRequest{Model: "wan2.5-t2i-preview", Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "unsafe prompt") {
		t.Fatalf("failed task: %v", err)
	}

	// A refused create is the vendor's answer, and nothing is polled.
	v = &text2imageVendor{createErr: `{"code":"InvalidParameter","message":"size is invalid"}`, answers: []string{"{}"}}
	a = fastPoll(NewDashScopeImageAdapter("d", v.server(t).URL, "k"), 5*time.Second)
	if _, err := a.Image(context.Background(), ImageRequest{Model: "wan2.2-t2i-plus", Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "size is invalid") || v.polls != 0 {
		t.Fatalf("refused create: %v (polls %d)", err, v.polls)
	}

	// Every result failing is no picture at all.
	v = &text2imageVendor{answers: []string{`{"output":{"task_id":"t-1","task_status":"SUCCEEDED","results":[{"code":"DataInspectionFailed","message":"unsafe"}]}}`}}
	a = fastPoll(NewDashScopeImageAdapter("d", v.server(t).URL, "k"), 5*time.Second)
	if _, err := a.Image(context.Background(), ImageRequest{Model: "wan2.2-t2i-plus", Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "DataInspectionFailed") {
		t.Fatalf("all results failed: %v", err)
	}

	// A task still running at the deadline is a failure, created once.
	v = &text2imageVendor{answers: []string{`{"output":{"task_id":"t-1","task_status":"RUNNING"}}`}}
	a = fastPoll(NewDashScopeImageAdapter("d", v.server(t).URL, "k"), 50*time.Millisecond)
	if _, err := a.Image(context.Background(), ImageRequest{Model: "wan2.2-t2i-flash", Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "not finished") || len(v.created) != 1 {
		t.Fatalf("deadline: %v (creates %d)", err, len(v.created))
	}

	// Input pictures never reach a text-to-image model.
	if _, err := a.Image(context.Background(), ImageRequest{Model: "wan2.2-t2i-flash", Prompt: "x", Images: []string{"https://e.com/a.png"}}); err == nil || len(v.created) != 1 {
		t.Fatalf("edit on a text-to-image model: %v (creates %d)", err, len(v.created))
	}
}

// Each model gets a canvas inside its documented bounds, whatever was asked.
func TestDashScopeText2ImageCanvas(t *testing.T) {
	for _, c := range []struct {
		model string
		req   ImageRequest
		want  string
	}{
		{"wan2.5-t2i-preview", ImageRequest{}, "1280*1280"},
		{"wan2.5-t2i-preview", ImageRequest{AspectRatio: "16:9"}, "1712*960"},
		{"wan2.5-t2i-preview", ImageRequest{AspectRatio: "9:16"}, "960*1712"},
		{"wan2.5-t2i-preview", ImageRequest{Size: "1024x1024"}, "1280*1280"},
		{"wan2.5-t2i-preview", ImageRequest{Size: "2048x2048"}, "1440*1440"},
		{"wan2.5-t2i-preview", ImageRequest{AspectRatio: "1:8"}, "640*2560"},
		{"wan2.2-t2i-plus", ImageRequest{}, "1024*1024"},
		{"wan2.2-t2i-plus", ImageRequest{AspectRatio: "16:9"}, "1360*768"},
		{"wan2.2-t2i-flash", ImageRequest{Size: "1920x1080"}, "1440*816"},
		{"wan2.2-t2i-flash", ImageRequest{AspectRatio: "1:4"}, "512*1440"},
		{"wan2.2-t2i-flash", ImageRequest{Size: "256x256"}, "512*512"},
	} {
		if got := wanText2ImageModels[c.model].canvas(c.req); got != c.want {
			t.Errorf("%s canvas(%+v) = %s, want %s", c.model, c.req, got, c.want)
		}
	}

	shapes := []ImageRequest{{}}
	for _, ar := range []string{"1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3", "4:5", "21:9", "1:4", "4:1", "1:10", "10:1"} {
		shapes = append(shapes, ImageRequest{AspectRatio: ar})
	}
	for _, size := range []string{"256x256", "512x512", "1024x1024", "1536x1024", "1024x1536", "1792x1024", "4096x4096", "4000x100"} {
		shapes = append(shapes, ImageRequest{Size: size})
	}
	for model, m := range wanText2ImageModels {
		for _, req := range shapes {
			got := m.canvas(req)
			w, h, _ := splitDims(got, "*")
			ratio := float64(w) / float64(h)
			switch {
			case m.minArea > 0 && w*h < m.minArea, m.maxArea > 0 && w*h > m.maxArea:
				t.Errorf("%s %+v: %s has %d px, outside [%d, %d]", model, req, got, w*h, m.minArea, m.maxArea)
			case m.minSide > 0 && (w < m.minSide || h < m.minSide), m.maxSide > 0 && (w > m.maxSide || h > m.maxSide):
				t.Errorf("%s %+v: %s has a side outside [%d, %d]", model, req, got, m.minSide, m.maxSide)
			case ratio < m.minRatio-0.02 || ratio > m.maxRatio+0.02:
				t.Errorf("%s %+v: %s is %.3f wide, outside [%.3f, %.3f]", model, req, got, ratio, m.minRatio, m.maxRatio)
			case w%16 != 0 || h%16 != 0:
				t.Errorf("%s %+v: %s is not in steps of 16", model, req, got)
			}
		}
	}
	// n is capped at the four a task makes.
	if n := dashScopeText2ImageBody(ImageRequest{Model: "wan2.2-t2i-flash"}, wanText2ImageModels["wan2.2-t2i-flash"], 9)["parameters"].(map[string]any)["n"]; n != 4 {
		t.Fatalf("n = %v", n)
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
