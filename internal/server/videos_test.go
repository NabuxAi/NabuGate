package server

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"nabugate/internal/agent"
	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// videoVendor is a video provider whose tasks finish when the test says so.
type videoVendor struct {
	mu     sync.Mutex
	states map[string]provider.VideoStatus
	got    []provider.VideoRequest
	polls  int
}

func (v *videoVendor) Name() string { return "dashscope-video" }
func (v *videoVendor) Chat(context.Context, provider.ChatRequest) (provider.ChatResponse, error) {
	return provider.ChatResponse{}, nil
}
func (v *videoVendor) CreateVideo(_ context.Context, req provider.VideoRequest) (provider.VideoJob, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.got = append(v.got, req)
	id := "task-" + strconv.Itoa(len(v.got))
	v.states[id] = provider.VideoStatus{State: provider.VideoProcessing}
	model, tier := req.Model, "720P"
	if req.ImageURL != "" {
		model = strings.Replace(model, "t2v", "i2v", 1)
	}
	if req.Resolution == "1080P" {
		tier = "1080P"
	}
	return provider.VideoJob{TaskID: id, Model: model, Resolution: tier, Seconds: 5}, nil
}
func (v *videoVendor) VideoStatus(_ context.Context, taskID string) (provider.VideoStatus, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.polls++
	return v.states[taskID], nil
}
func (v *videoVendor) set(taskID string, st provider.VideoStatus) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.states[taskID] = st
}

var videoPrices = map[string]usage.Price{
	"dashscope-video/wan2.6-t2v":       {PerMinute: 6},
	"dashscope-video/wan2.6-t2v@1080P": {PerMinute: 9},
	"dashscope-video/wan2.6-i2v":       {PerMinute: 6},
}

func videoServer(t *testing.T, vendor *videoVendor, statePath string) (*httptest.Server, *usage.Tracker) {
	t.Helper()
	r := router.New(map[string]provider.Adapter{"dashscope-video": vendor}, nil, nil, nil, nil, nil, nil, discardLogger())
	r.SetVideos(map[string]config.ModelRoute{"nabu-video": {Primary: config.Target{Provider: "dashscope-video", Model: "wan2.6-t2v"}}})
	keys := policy.New(nil, []policy.KeyConfig{
		{Key: "gen-key", Project: "nabugen", Allow: []string{"nabu-*"}, RateLimit: 10000},
		{Key: "other-key", Project: "nabugen", Allow: []string{"nabu-*"}, RateLimit: 10000},
		{Key: "chat-only", Project: "other", Allow: []string{"nabu-fast"}, RateLimit: 10000},
	})
	tracker := usage.New(videoPrices)
	srv := New(r, keys, tracker, agent.NewRegistry(), discardLogger())
	if statePath != "" {
		if err := srv.SetVideoStateFile(statePath); err != nil {
			t.Fatal(err)
		}
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, tracker
}

type videoAnswer struct {
	status int
	cost   string
	body   map[string]any
}

func videoCall(t *testing.T, ts *httptest.Server, method, path, key, body string) videoAnswer {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return videoAnswer{status: resp.StatusCode, cost: resp.Header.Get(costHeader), body: out}
}

func byModel(t *usage.Tracker) map[string]usage.Stat {
	_, m := t.Snapshot()
	return m
}

func dataURL(body map[string]any) string {
	data, _ := body["data"].([]any)
	if len(data) != 1 {
		return ""
	}
	first, _ := data[0].(map[string]any)
	s, _ := first["url"].(string)
	return s
}

func TestVideoJobLifecycleAndSingleBilling(t *testing.T) {
	vendor := &videoVendor{states: map[string]provider.VideoStatus{}}
	ts, tracker := videoServer(t, vendor, "")

	created := videoCall(t, ts, http.MethodPost, "/v1/videos/generations", "gen-key",
		`{"model":"nabu-video","prompt":"a fox in snow","size":"1280x720","aspect_ratio":"16:9","seconds":5}`)
	id, _ := created.body["id"].(string)
	if created.status != http.StatusOK || created.body["status"] != "processing" || !strings.HasPrefix(id, "video_") || created.cost != "" {
		t.Fatalf("create: %+v", created)
	}
	if strings.Contains(id, "task-") {
		t.Fatalf("job id %q leaks the vendor task id", id)
	}
	if got := vendor.got[0]; got.Prompt != "a fox in snow" || got.Size != "1280x720" || got.AspectRatio != "16:9" || got.Seconds != 5 || got.ImageURL != "" {
		t.Fatalf("vendor got %+v", got)
	}

	poll := func(key string) videoAnswer {
		return videoCall(t, ts, http.MethodGet, "/v1/videos/generations/"+id, key, "")
	}
	if a := poll("gen-key"); a.status != http.StatusOK || a.body["status"] != "processing" || a.body["id"] != id || a.cost != "" {
		t.Fatalf("running poll: %+v", a)
	}
	// Another key — even one on the same project — cannot see the job.
	if a := poll("other-key"); a.status != http.StatusNotFound {
		t.Fatalf("foreign key polled the job: %+v", a)
	}
	if a := videoCall(t, ts, http.MethodGet, "/v1/videos/generations/video_doesnotexist", "gen-key", ""); a.status != http.StatusNotFound {
		t.Fatalf("unknown id: %+v", a)
	}

	vendor.set("task-1", provider.VideoStatus{State: provider.VideoSucceeded, URL: "https://oss/v.mp4", Seconds: 5, Resolution: "720P"})
	done := poll("gen-key")
	if done.status != http.StatusOK || done.body["status"] != "completed" || dataURL(done.body) != "https://oss/v.mp4" {
		t.Fatalf("done poll: %+v", done)
	}
	if cost, _ := strconv.ParseFloat(done.cost, 64); math.Abs(cost-0.50) > 1e-9 {
		t.Fatalf("5 s at 720P charged %q, want 0.50", done.cost)
	}

	// Later polls answer from the record: no vendor call, no second charge.
	pollsBefore := vendor.polls
	for i := 0; i < 3; i++ {
		again := poll("gen-key")
		if again.body["status"] != "completed" || dataURL(again.body) != "https://oss/v.mp4" || again.cost != "" {
			t.Fatalf("repeat poll %d: %+v", i, again)
		}
	}
	if vendor.polls != pollsBefore {
		t.Fatalf("a finished job was polled upstream again")
	}
	if a := poll("other-key"); a.status != http.StatusNotFound {
		t.Fatalf("foreign key read a finished job: %+v", a)
	}
	stat := byModel(tracker)["dashscope-video/wan2.6-t2v"]
	if stat.Requests != 1 || math.Abs(stat.CostUSD-0.50) > 1e-9 || stat.AudioSeconds != 5 {
		t.Fatalf("metered %+v; want one request of 5 s at $0.50", stat)
	}
}

func TestVideoFailedJobIsNotBilled(t *testing.T) {
	vendor := &videoVendor{states: map[string]provider.VideoStatus{}}
	ts, tracker := videoServer(t, vendor, "")
	created := videoCall(t, ts, http.MethodPost, "/v1/videos/generations", "gen-key", `{"model":"nabu-video","prompt":"x"}`)
	id := created.body["id"].(string)

	vendor.set("task-1", provider.VideoStatus{State: provider.VideoFailed, Message: "DataInspectionFailed unsafe input"})
	for i := 0; i < 2; i++ {
		a := videoCall(t, ts, http.MethodGet, "/v1/videos/generations/"+id, "gen-key", "")
		if a.status != http.StatusOK || a.body["status"] != "failed" || a.body["message"] != "DataInspectionFailed unsafe input" || a.cost != "" {
			t.Fatalf("failed poll %d: %+v", i, a)
		}
	}
	if len(byModel(tracker)) != 0 {
		t.Fatalf("a failed video was metered: %+v", byModel(tracker))
	}
}

// Many polls racing on the moment the task finishes still bill it once.
func TestVideoConcurrentPollsBillOnce(t *testing.T) {
	vendor := &videoVendor{states: map[string]provider.VideoStatus{}}
	ts, tracker := videoServer(t, vendor, "")
	id := videoCall(t, ts, http.MethodPost, "/v1/videos/generations", "gen-key", `{"model":"nabu-video","prompt":"x","resolution":"1080P"}`).body["id"].(string)
	vendor.set("task-1", provider.VideoStatus{State: provider.VideoSucceeded, URL: "https://oss/v.mp4", Seconds: 10, Resolution: "1080P"})

	var wg sync.WaitGroup
	var mu sync.Mutex
	charged := 0
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := videoCall(t, ts, http.MethodGet, "/v1/videos/generations/"+id, "gen-key", "")
			if a.cost != "" {
				mu.Lock()
				charged++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	stat := byModel(tracker)["dashscope-video/wan2.6-t2v@1080P"]
	if charged != 1 || stat.Requests != 1 || math.Abs(stat.CostUSD-1.50) > 1e-9 {
		t.Fatalf("charged on %d polls; metered %+v; want once, 10 s at 1080P = $1.50", charged, stat)
	}
}

// The registry outlives a restart: a job started before a redeploy can still
// be polled, and one already billed is not billed again.
func TestVideoJobsSurviveARestart(t *testing.T) {
	path := t.TempDir() + "/video-jobs.json"
	vendor := &videoVendor{states: map[string]provider.VideoStatus{}}
	ts, _ := videoServer(t, vendor, path)
	running := videoCall(t, ts, http.MethodPost, "/v1/videos/generations", "gen-key", `{"model":"nabu-video","prompt":"a"}`).body["id"].(string)
	billed := videoCall(t, ts, http.MethodPost, "/v1/videos/generations", "gen-key", `{"model":"nabu-video","prompt":"b"}`).body["id"].(string)
	vendor.set("task-2", provider.VideoStatus{State: provider.VideoSucceeded, URL: "https://oss/b.mp4", Seconds: 5})
	if a := videoCall(t, ts, http.MethodGet, "/v1/videos/generations/"+billed, "gen-key", ""); a.cost == "" {
		t.Fatalf("first finished poll was not billed: %+v", a)
	}
	ts.Close()

	ts2, tracker2 := videoServer(t, vendor, path)
	if a := videoCall(t, ts2, http.MethodGet, "/v1/videos/generations/"+billed, "gen-key", ""); a.body["status"] != "completed" || a.cost != "" {
		t.Fatalf("billed job after restart: %+v", a)
	}
	vendor.set("task-1", provider.VideoStatus{State: provider.VideoSucceeded, URL: "https://oss/a.mp4", Seconds: 5})
	if a := videoCall(t, ts2, http.MethodGet, "/v1/videos/generations/"+running, "gen-key", ""); dataURL(a.body) != "https://oss/a.mp4" || a.cost == "" {
		t.Fatalf("running job after restart: %+v", a)
	}
	if a := videoCall(t, ts2, http.MethodGet, "/v1/videos/generations/"+running, "other-key", ""); a.status != http.StatusNotFound {
		t.Fatalf("ownership lost in the restart: %+v", a)
	}
	if stat := byModel(tracker2)["dashscope-video/wan2.6-t2v"]; stat.Requests != 1 {
		t.Fatalf("after restart metered %+v; want only the job that finished after it", stat)
	}
}

func TestVideoCreateRequests(t *testing.T) {
	vendor := &videoVendor{states: map[string]provider.VideoStatus{}}
	ts, _ := videoServer(t, vendor, "")

	// Image-to-video: the first frame reaches the vendor, and the job runs on
	// the i2v model.
	a := videoCall(t, ts, http.MethodPost, "/v1/videos/generations", "gen-key",
		`{"model":"nabu-video","prompt":"wave","image_url":"data:image/png;base64,iVBORw0KGgo=","seconds":"10"}`)
	if a.status != http.StatusOK || vendor.got[0].ImageURL != "data:image/png;base64,iVBORw0KGgo=" || vendor.got[0].Seconds != 10 {
		t.Fatalf("i2v create: %+v, vendor got %+v", a, vendor.got)
	}

	for _, c := range []struct {
		name, key, body string
		status          int
		msg             string
	}{
		{"no prompt", "gen-key", `{"model":"nabu-video"}`, 400, "'prompt' are required"},
		{"no model", "gen-key", `{"prompt":"x"}`, 400, "'model'"},
		{"http first frame", "gen-key", `{"model":"nabu-video","prompt":"x","image_url":"http://e.com/f.png"}`, 400, "image_url: must be an https:// URL"},
		{"gif first frame", "gen-key", `{"model":"nabu-video","prompt":"x","image_url":"data:image/gif;base64,R0lGOD=="}`, 400, "is not accepted"},
		{"bad seconds", "gen-key", `{"model":"nabu-video","prompt":"x","seconds":"long"}`, 400, "'seconds'"},
		{"unknown alias", "gen-key", `{"model":"nabu-nope","prompt":"x"}`, 400, "unknown video alias"},
		{"alias not allowed", "chat-only", `{"model":"nabu-video","prompt":"x"}`, 403, ""},
	} {
		a := videoCall(t, ts, http.MethodPost, "/v1/videos/generations", c.key, c.body)
		msg, _ := a.body["error"].(map[string]any)["message"].(string)
		if a.status != c.status || !strings.Contains(msg, c.msg) {
			t.Errorf("%s: %d %q; want %d containing %q", c.name, a.status, msg, c.status, c.msg)
		}
	}
}

func TestModelsListsTheVideoAlias(t *testing.T) {
	ts, _ := videoServer(t, &videoVendor{states: map[string]provider.VideoStatus{}}, "")
	a := videoCall(t, ts, http.MethodGet, "/v1/models", "gen-key", "")
	data, _ := a.body["data"].([]any)
	for _, m := range data {
		if m.(map[string]any)["id"] == "nabu-video" {
			return
		}
	}
	t.Fatalf("/v1/models = %v; nabu-video missing", a.body)
}
