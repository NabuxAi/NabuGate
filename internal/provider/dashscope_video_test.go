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
	for _, c := range []struct {
		name    string
		req     VideoRequest
		want    string
		tier    string
		seconds int
	}{
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
		{"image-to-video switches model and sends a tier, not a size",
			VideoRequest{Model: "wan2.6-t2v", Prompt: "wave", ImageURL: "https://example.com/f.png", AspectRatio: "9:16", Seconds: 7},
			`{"input":{"img_url":"https://example.com/f.png","prompt":"wave"},"model":"wan2.6-i2v","parameters":{"duration":5,"resolution":"720P"}}`, "720P", 5},
	} {
		body, _, tier, secs := dashScopeVideoBody(c.req)
		got, _ := json.Marshal(body)
		if string(got) != c.want || tier != c.tier || secs != c.seconds {
			t.Errorf("%s:\n got %s (%s, %ds)\nwant %s (%s, %ds)", c.name, got, tier, secs, c.want, c.tier, c.seconds)
		}
	}
}

func TestNearestDuration(t *testing.T) {
	for in, want := range map[float64]int{0: 5, 1: 5, 5: 5, 7: 5, 7.5: 10, 8: 10, 12: 10, 12.5: 15, 15: 15, 30: 15} {
		if got := nearestDuration(in, dashScopeVideoDurations); got != want {
			t.Errorf("nearestDuration(%v) = %d, want %d", in, got, want)
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

// A request the vendor calls malformed is the caller's to fix, and a refused
// inline first frame says what to send instead.
func TestDashScopeVideoInvalidRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"InvalidParameter","message":"img_url is invalid"}`))
	}))
	defer srv.Close()
	a := NewDashScopeVideoAdapter("dashscope-video", srv.URL, "k")
	_, err := a.CreateVideo(context.Background(), VideoRequest{Model: "wan2.6-t2v", ImageURL: "data:image/png;base64,iVBORw0KGgo="})
	if !errors.Is(err, ErrInvalidVideoRequest) || !strings.Contains(err.Error(), "img_url is invalid") || !strings.Contains(err.Error(), "https:// URL instead") {
		t.Fatalf("err = %v", err)
	}
}
