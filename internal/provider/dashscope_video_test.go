package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashScopeVideoBodyMapping(t *testing.T) {
	const frame = "https://example.com/f.png"
	for _, c := range []struct {
		name    string
		req     VideoRequest
		want    string
		tier    string
		seconds int
	}{
		// Wan 2.6 (nabu-video).
		{"defaults: 16:9, 720P, 5 s",
			VideoRequest{Model: "wan2.6-t2v", Prompt: "a fox"},
			`{"input":{"prompt":"a fox"},"model":"wan2.6-t2v","parameters":{"duration":5,"size":"1280*720"}}`, "720P", 5},
		{"vertical by aspect ratio",
			VideoRequest{Model: "wan2.6-t2v", Prompt: "p", AspectRatio: "9:16", Seconds: 10},
			`{"input":{"prompt":"p"},"model":"wan2.6-t2v","parameters":{"duration":10,"size":"720*1280"}}`, "720P", 10},
		{"1080P square by size",
			VideoRequest{Model: "wan2.6-t2v", Prompt: "p", Size: "1440x1440", Seconds: 15},
			`{"input":{"prompt":"p"},"model":"wan2.6-t2v","parameters":{"duration":15,"size":"1440*1440"}}`, "1080P", 15},
		{"1080P from size, shape from aspect ratio",
			VideoRequest{Model: "wan2.6-t2v", Prompt: "p", Size: "1920x1080", AspectRatio: "9:16"},
			`{"input":{"prompt":"p"},"model":"wan2.6-t2v","parameters":{"duration":5,"size":"1080*1920"}}`, "1080P", 5},
		{"explicit resolution, unusual ratio to its nearest, seconds clamped",
			VideoRequest{Model: "wan2.6-t2v", Prompt: "p", Resolution: "1080p", AspectRatio: "21:9", Seconds: 60, NegativePrompt: "text"},
			`{"input":{"negative_prompt":"text","prompt":"p"},"model":"wan2.6-t2v","parameters":{"duration":15,"size":"1920*1080"}}`, "1080P", 15},
		{"4:3 has its own canvas at 720P",
			VideoRequest{Model: "wan2.6-t2v", Prompt: "p", AspectRatio: "4:3", Seconds: 7},
			`{"input":{"prompt":"p"},"model":"wan2.6-t2v","parameters":{"duration":7,"size":"1088*832"}}`, "720P", 7},
		{"480P asked of wan2.6 renders at 720P; a second is the shortest offered",
			VideoRequest{Model: "wan2.6-t2v", Prompt: "p", Resolution: "480P", Seconds: 1},
			`{"input":{"prompt":"p"},"model":"wan2.6-t2v","parameters":{"duration":2,"size":"1280*720"}}`, "720P", 2},
		{"image-to-video runs the alias's image model with a tier, not a size",
			VideoRequest{Model: "wan2.6-t2v", ImageModel: "wan2.6-i2v", Prompt: "wave", ImageURL: frame, AspectRatio: "9:16", Seconds: 7},
			`{"input":{"img_url":"https://example.com/f.png","prompt":"wave"},"model":"wan2.6-i2v","parameters":{"duration":7,"resolution":"720P"}}`, "720P", 7},

		// Wan 2.5 preview (nabu-video-wan25): 480/720/1080P, 5 or 10 s.
		{"wan2.5 at 480P, vertical, 7 s to 5",
			VideoRequest{Model: "wan2.5-t2v-preview", Prompt: "p", Resolution: "480P", AspectRatio: "9:16", Seconds: 7},
			`{"input":{"prompt":"p"},"model":"wan2.5-t2v-preview","parameters":{"duration":5,"size":"480*832"}}`, "480P", 5},
		{"wan2.5 tier from a 480P size",
			VideoRequest{Model: "wan2.5-t2v-preview", Prompt: "p", Size: "624x624", Seconds: 8},
			`{"input":{"prompt":"p"},"model":"wan2.5-t2v-preview","parameters":{"duration":10,"size":"624*624"}}`, "480P", 10},
		{"wan2.5 from a photo at 1080P, 12 s to 10",
			VideoRequest{Model: "wan2.5-t2v-preview", ImageModel: "wan2.5-i2v-preview", ImageURL: frame, Resolution: "1080P", Seconds: 12},
			`{"input":{"img_url":"https://example.com/f.png"},"model":"wan2.5-i2v-preview","parameters":{"duration":10,"resolution":"1080P"}}`, "1080P", 10},

		// Wan 2.2 Plus (nabu-video-wan22): 480P or 1080P, fixed 5 s, so no
		// duration is sent.
		{"wan2.2 default: 720P asked is a tie, which goes up to 1080P",
			VideoRequest{Model: "wan2.2-t2v-plus", Prompt: "p", Seconds: 10},
			`{"input":{"prompt":"p"},"model":"wan2.2-t2v-plus","parameters":{"size":"1920*1080"}}`, "1080P", 5},
		{"wan2.2 at 480P, square",
			VideoRequest{Model: "wan2.2-t2v-plus", Prompt: "p", Resolution: "480p", AspectRatio: "1:1"},
			`{"input":{"prompt":"p"},"model":"wan2.2-t2v-plus","parameters":{"size":"624*624"}}`, "480P", 5},
		{"wan2.2 from a photo",
			VideoRequest{Model: "wan2.2-t2v-plus", ImageModel: "wan2.2-i2v-plus", ImageURL: frame, Prompt: "p", Resolution: "480P"},
			`{"input":{"img_url":"https://example.com/f.png","prompt":"p"},"model":"wan2.2-i2v-plus","parameters":{"resolution":"480P"}}`, "480P", 5},

		// nabu-video-fast: Wan 2.1 Turbo from text, Wan 2.2 Flash from a photo.
		{"fast from text: 1080P asked renders at 720P",
			VideoRequest{Model: "wan2.1-t2v-turbo", ImageModel: "wan2.2-i2v-flash", Prompt: "p", Resolution: "1080P", AspectRatio: "1:1"},
			`{"input":{"prompt":"p"},"model":"wan2.1-t2v-turbo","parameters":{"size":"960*960"}}`, "720P", 5},
		{"fast from a photo runs wan2.2-i2v-flash, not a renamed turbo",
			VideoRequest{Model: "wan2.1-t2v-turbo", ImageModel: "wan2.2-i2v-flash", ImageURL: frame, Prompt: "p", Size: "1920x1080", Seconds: 10},
			`{"input":{"img_url":"https://example.com/f.png","prompt":"p"},"model":"wan2.2-i2v-flash","parameters":{"resolution":"720P"}}`, "720P", 5},

		// Wan 2.1 Plus (nabu-video-wan21): 720P only.
		{"wan2.1 plus: 480P asked is 720P",
			VideoRequest{Model: "wan2.1-t2v-plus", ImageModel: "wan2.1-i2v-plus", Prompt: "p", Resolution: "480P", AspectRatio: "3:4"},
			`{"input":{"prompt":"p"},"model":"wan2.1-t2v-plus","parameters":{"size":"832*1088"}}`, "720P", 5},
		{"wan2.1 plus from a photo",
			VideoRequest{Model: "wan2.1-t2v-plus", ImageModel: "wan2.1-i2v-plus", ImageURL: frame, Resolution: "1080P"},
			`{"input":{"img_url":"https://example.com/f.png"},"model":"wan2.1-i2v-plus","parameters":{"resolution":"720P"}}`, "720P", 5},

		// wan2.1-i2v-turbo, whose lengths are 3, 4 or 5 s.
		{"i2v turbo as the alias's own model: 3.4 s to 3",
			VideoRequest{Model: "wan2.1-i2v-turbo", ImageURL: frame, Prompt: "p", Resolution: "480P", Seconds: 3.4},
			`{"input":{"img_url":"https://example.com/f.png","prompt":"p"},"model":"wan2.1-i2v-turbo","parameters":{"duration":3,"resolution":"480P"}}`, "480P", 3},
	} {
		body, _, tier, secs, err := dashScopeVideoBody(c.req)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		got, _ := json.Marshal(body)
		if string(got) != c.want || tier != c.tier || secs != c.seconds {
			t.Errorf("%s:\n got %s (%s, %ds)\nwant %s (%s, %ds)", c.name, got, tier, secs, c.want, c.tier, c.seconds)
		}
	}
}

// A request the alias cannot serve is the caller's to fix; a model the adapter
// has no table for is a configuration fault, and nothing is sent.
func TestDashScopeVideoBodyRefusals(t *testing.T) {
	_, _, _, _, err := dashScopeVideoBody(VideoRequest{Model: "wan2.1-t2v-turbo", ImageURL: "https://e.com/f.png"})
	if !errors.Is(err, ErrInvalidVideoRequest) || !strings.Contains(err.Error(), "no image-to-video model") {
		t.Errorf("photo on a text-only route: %v", err)
	}
	_, _, _, _, err = dashScopeVideoBody(VideoRequest{Model: "wan2.2-i2v-flash", Prompt: "p"})
	if !errors.Is(err, ErrInvalidVideoRequest) || !strings.Contains(err.Error(), "send image_url") {
		t.Errorf("text on an image-to-video model: %v", err)
	}
	_, _, _, _, err = dashScopeVideoBody(VideoRequest{Model: "wanx2.1-vace-plus", Prompt: "p"})
	if err == nil || errors.Is(err, ErrInvalidVideoRequest) {
		t.Errorf("unknown model: %v", err)
	}
	_, _, _, _, err = dashScopeVideoBody(VideoRequest{Model: "wan2.6-t2v", ImageModel: "wan2.6-i2v-typo", ImageURL: "https://e.com/f.png"})
	if err == nil || !strings.Contains(err.Error(), "wan2.6-i2v-typo") {
		t.Errorf("unknown image model: %v", err)
	}
}

func TestNearestDuration(t *testing.T) {
	for _, c := range []struct {
		offered []int
		cases   map[float64]int
	}{
		{[]int{5, 10, 15}, map[float64]int{0: 5, 1: 5, 5: 5, 7: 5, 7.5: 10, 8: 10, 12: 10, 12.5: 15, 15: 15, 30: 15}},
		{secondsFrom(2, 15), map[float64]int{0: 5, -1: 5, 1: 2, 7: 7, 7.5: 8, 14.2: 14, 60: 15}},
		{[]int{5, 10}, map[float64]int{0: 5, 7: 5, 7.5: 10, 20: 10}},
		{[]int{3, 4, 5}, map[float64]int{0: 5, 2: 3, 3.4: 3, 3.5: 4, 9: 5}},
		{[]int{5}, map[float64]int{0: 5, 15: 5}},
	} {
		for in, want := range c.cases {
			if got := nearestDuration(in, c.offered); got != want {
				t.Errorf("nearestDuration(%v, %v) = %d, want %d", in, c.offered, got, want)
			}
		}
	}
}

func TestNearestTierAndTierOfDims(t *testing.T) {
	for _, c := range []struct {
		want    string
		offered []string
		got     string
	}{
		{"720P", []string{"480P", "1080P"}, "1080P"},
		{"480P", []string{"480P", "1080P"}, "480P"},
		{"1080P", []string{"480P", "720P"}, "720P"},
		{"480P", []string{"720P"}, "720P"},
		{"480P", []string{"720P", "1080P"}, "720P"},
		{"1080P", []string{"480P", "720P", "1080P"}, "1080P"},
	} {
		if got := nearestTier(c.want, c.offered); got != c.got {
			t.Errorf("nearestTier(%s, %v) = %s, want %s", c.want, c.offered, got, c.got)
		}
	}
	for dims, want := range map[string]string{
		"832*480": "480P", "624*624": "480P", "480x832": "480P",
		"1280x720": "720P", "960*960": "720P", "1088*832": "720P",
		"1920*1080": "1080P", "1440*1440": "1080P", "1632*1248": "1080P", "1920_1080": "1080P",
	} {
		w, h, ok := videoDims(dims)
		if got := tierOfDims(w, h); !ok || got != want {
			t.Errorf("tierOfDims(%s) = %s (%v), want %s", dims, got, ok, want)
		}
	}
}

// Every table entry is internally consistent: tiers it can draw, at least one
// length, and the shipped pairs are text and image models respectively.
func TestWanVideoModelTable(t *testing.T) {
	for name, spec := range wanVideoModels {
		if len(spec.tiers) == 0 || len(spec.durations) == 0 {
			t.Errorf("%s has no tiers or durations", name)
		}
		for _, tier := range spec.tiers {
			if _, ok := wanVideoCanvases[tier]; !ok {
				t.Errorf("%s offers unknown tier %s", name, tier)
			}
		}
		if spec.imageToVideo != strings.Contains(name, "i2v") {
			t.Errorf("%s: imageToVideo = %v", name, spec.imageToVideo)
		}
	}
	for _, missing := range []string{"wanx2.1-kf2v-plus", "wanx2.1-vace-plus", "wan2.2-s2v"} {
		if _, _, ok := WanVideoModel(missing); ok {
			t.Errorf("%s is not on the key and must not be in the table", missing)
		}
	}
}

func TestDashScopeVideoCreateAndPoll(t *testing.T) {
	var created map[string]any
	status := `{"output":{"task_id":"t-1","task_status":"RUNNING"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"InvalidApiKey","message":"bad key"}`))
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/services/aigc/video-generation/video-synthesis":
			if r.Header.Get("X-DashScope-Async") != "enable" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"code":"InvalidParameter","message":"sync not supported"}`))
				return
			}
			_ = json.NewDecoder(r.Body).Decode(&created)
			_, _ = w.Write([]byte(`{"output":{"task_id":"t-1","task_status":"PENDING"},"request_id":"r"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/tasks/t-1":
			_, _ = w.Write([]byte(status))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	a := NewDashScopeVideoAdapter("dashscope-video", srv.URL, "k")
	var _ VideoAdapter = a
	job, err := a.CreateVideo(context.Background(), VideoRequest{Model: "wan2.6-t2v", Prompt: "a fox", Seconds: 5})
	if err != nil {
		t.Fatalf("CreateVideo: %v", err)
	}
	if job.TaskID != "t-1" || job.Model != "wan2.6-t2v" || job.Resolution != "720P" || job.Seconds != 5 || created["model"] != "wan2.6-t2v" {
		t.Fatalf("job = %+v, body = %v", job, created)
	}

	for _, c := range []struct {
		body string
		want VideoStatus
	}{
		{`{"output":{"task_id":"t-1","task_status":"PENDING"}}`, VideoStatus{State: VideoProcessing}},
		{`{"output":{"task_id":"t-1","task_status":"RUNNING"}}`, VideoStatus{State: VideoProcessing}},
		{`{"output":{"task_id":"t-1","task_status":"SUCCEEDED","video_url":"https://oss/v.mp4"},"usage":{"duration":5,"SR":720,"output_video_duration":5}}`,
			VideoStatus{State: VideoSucceeded, URL: "https://oss/v.mp4", Seconds: 5, Resolution: "720P"}},
		{`{"output":{"task_id":"t-1","task_status":"SUCCEEDED","video_url":"https://oss/v.mp4"},"usage":{"duration":10,"SR":"1080"}}`,
			VideoStatus{State: VideoSucceeded, URL: "https://oss/v.mp4", Seconds: 10, Resolution: "1080P"}},
		{`{"output":{"task_id":"t-1","task_status":"SUCCEEDED","video_url":"https://oss/v.mp4"},"usage":{"duration":5,"SR":480,"output_video_duration":5}}`,
			VideoStatus{State: VideoSucceeded, URL: "https://oss/v.mp4", Seconds: 5, Resolution: "480P"}},
		// Wan 2.5 and earlier spell usage video_duration and video_ratio.
		{`{"output":{"task_id":"t-1","task_status":"SUCCEEDED","video_url":"https://oss/v.mp4"},"usage":{"video_duration":10,"video_ratio":"832*480","video_count":1}}`,
			VideoStatus{State: VideoSucceeded, URL: "https://oss/v.mp4", Seconds: 10, Resolution: "480P"}},
		{`{"output":{"task_id":"t-1","task_status":"SUCCEEDED","video_url":"https://oss/v.mp4"},"usage":{"video_duration":5,"video_ratio":"1920*1080","video_count":1}}`,
			VideoStatus{State: VideoSucceeded, URL: "https://oss/v.mp4", Seconds: 5, Resolution: "1080P"}},
		{`{"output":{"task_id":"t-1","task_status":"SUCCEEDED","video_url":"https://oss/v.mp4"},"usage":{"video_duration":5,"video_ratio":"960*960","video_count":1}}`,
			VideoStatus{State: VideoSucceeded, URL: "https://oss/v.mp4", Seconds: 5, Resolution: "720P"}},
		{`{"output":{"task_id":"t-1","task_status":"FAILED","code":"DataInspectionFailed","message":"unsafe input"}}`,
			VideoStatus{State: VideoFailed, Message: "DataInspectionFailed unsafe input"}},
		{`{"output":{"task_id":"t-1","task_status":"UNKNOWN"}}`, VideoStatus{State: VideoFailed, Message: "video task unknown"}},
		{`{"output":{"task_id":"t-1","task_status":"CANCELED"}}`, VideoStatus{State: VideoFailed, Message: "video task canceled"}},
	} {
		status = c.body
		got, err := a.VideoStatus(context.Background(), "t-1")
		if err != nil || got != c.want {
			t.Errorf("status for %s = %+v (%v), want %+v", c.body, got, err, c.want)
		}
	}

	if _, err := NewDashScopeVideoAdapter("d", srv.URL, "wrong").VideoStatus(context.Background(), "t-1"); err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("want the upstream message on a failed poll, got %v", err)
	}
}

// The fast alias's photo path starts the task on its own image model, and the
// job says so, which is what it is billed under.
func TestDashScopeVideoCreateFromAPhotoOnTheFastPair(t *testing.T) {
	var created map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&created)
		_, _ = w.Write([]byte(`{"output":{"task_id":"t-9","task_status":"PENDING"}}`))
	}))
	defer srv.Close()
	job, err := NewDashScopeVideoAdapter("dashscope-video", srv.URL, "k").CreateVideo(context.Background(),
		VideoRequest{Model: "wan2.1-t2v-turbo", ImageModel: "wan2.2-i2v-flash", ImageURL: "https://example.com/f.png", Resolution: "480P"})
	if err != nil {
		t.Fatal(err)
	}
	if job.Model != "wan2.2-i2v-flash" || job.Resolution != "480P" || job.Seconds != 5 || created["model"] != "wan2.2-i2v-flash" {
		t.Fatalf("job %+v, body %v", job, created)
	}
	if _, sent := created["parameters"].(map[string]any)["duration"]; sent {
		t.Fatalf("a fixed-length model was sent a duration: %v", created)
	}

	// Nothing reaches the vendor when the alias cannot serve the request.
	created = nil
	if _, err := NewDashScopeVideoAdapter("dashscope-video", srv.URL, "k").CreateVideo(context.Background(),
		VideoRequest{Model: "wan2.1-t2v-turbo", ImageURL: "https://example.com/f.png"}); !errors.Is(err, ErrInvalidVideoRequest) || created != nil {
		t.Fatalf("err %v, sent %v", err, created)
	}
}

// A request the vendor calls malformed is the caller's to fix, and a refused
// inline first frame says what to send instead.
func TestDashScopeVideoInvalidRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"InvalidParameter","message":"img_url is invalid"}`))
	}))
	defer srv.Close()
	a := NewDashScopeVideoAdapter("dashscope-video", srv.URL, "k")
	_, err := a.CreateVideo(context.Background(), VideoRequest{Model: "wan2.6-t2v", ImageModel: "wan2.6-i2v", ImageURL: "data:image/png;base64,iVBORw0KGgo="})
	if !errors.Is(err, ErrInvalidVideoRequest) || !strings.Contains(err.Error(), "img_url is invalid") || !strings.Contains(err.Error(), "https:// URL instead") {
		t.Fatalf("err = %v", err)
	}
}
