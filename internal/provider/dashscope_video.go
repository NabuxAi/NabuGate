package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// DashScopeVideoAdapter serves Alibaba's Wan video models (wan2.6-t2v and
// wan2.6-i2v) from Model Studio's native, asynchronous video-synthesis API:
// CreateVideo submits a task with X-DashScope-Async and returns its id, and
// VideoStatus reads GET /tasks/{id}. A finished task carries a signed OSS URL
// (valid about 24 hours) and the duration and tier the vendor bills for.
//
// The alias names the text-to-video model. A request with a first frame runs
// on the image-to-video sibling instead, found by swapping "t2v" for "i2v" in
// the name, so one alias serves both the way callers expect.
type DashScopeVideoAdapter struct {
	name    string
	baseURL string
	apiKey  string
}

// NewDashScopeVideoAdapter builds the adapter. baseURL defaults to the
// international endpoint; the mainland one rejects international keys.
func NewDashScopeVideoAdapter(name, baseURL, apiKey string) *DashScopeVideoAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://dashscope-intl.aliyuncs.com/api/v1"
	}
	return &DashScopeVideoAdapter{name: name, baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey}
}

func (a *DashScopeVideoAdapter) Name() string { return a.name }

// Chat is unsupported: chat goes through the dashscope openai-wire provider.
func (a *DashScopeVideoAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, fmt.Errorf("%s: chat is not supported by the DashScope video provider", a.name)
}

// dashScopeVideoCanvases are the text-to-video sizes Wan 2.6 draws, per tier
// and aspect ratio.
var dashScopeVideoCanvases = map[string]map[string]string{
	"720P":  {"16:9": "1280*720", "9:16": "720*1280", "1:1": "960*960"},
	"1080P": {"16:9": "1920*1080", "9:16": "1080*1920", "1:1": "1440*1440"},
}

// dashScopeVideoDurations are the lengths, in seconds, Wan 2.6 renders.
var dashScopeVideoDurations = []int{5, 10, 15}

// dashScopeVideoBody maps a request onto Wan's: the model (i2v with a first
// frame), the input and the parameters, plus the tier and duration chosen.
func dashScopeVideoBody(req VideoRequest) (body map[string]any, model, tier string, seconds int) {
	tier = videoTier(req)
	seconds = nearestDuration(req.Seconds, dashScopeVideoDurations)
	model = req.Model

	input := map[string]any{}
	if p := strings.TrimSpace(req.Prompt); p != "" {
		input["prompt"] = p
	}
	if neg := strings.TrimSpace(req.NegativePrompt); neg != "" {
		input["negative_prompt"] = neg
	}
	params := map[string]any{"duration": seconds}
	if img := strings.TrimSpace(req.ImageURL); img != "" {
		// Image-to-video follows the first frame's shape; only the tier is
		// chosen.
		model = imageToVideoModel(model)
		input["img_url"] = img
		params["resolution"] = tier
	} else {
		params["size"] = dashScopeVideoCanvases[tier][videoAspect(req)]
	}
	return map[string]any{"model": model, "input": input, "parameters": params}, model, tier, seconds
}

// imageToVideoModel is the image-to-video sibling of a text-to-video model
// (wan2.6-t2v → wan2.6-i2v). A name without "t2v" is taken to be one already.
func imageToVideoModel(model string) string {
	return strings.Replace(model, "t2v", "i2v", 1)
}

// videoTier picks 720P or 1080P: an explicit resolution, else a tier written
// as the size ("1080p"), else a WxH size whose long side is past 720p's 1280,
// else 720P.
func videoTier(req VideoRequest) string {
	for _, v := range []string{req.Resolution, req.Size} {
		switch strings.ToUpper(strings.TrimSpace(v)) {
		case "1080P", "1080":
			return "1080P"
		case "720P", "720":
			return "720P"
		}
	}
	if w, h, ok := videoDims(req.Size); ok && max(w, h) > 1280 {
		return "1080P"
	}
	return "720P"
}

// videoAspect picks the canvas shape nearest the request's aspect ratio, else
// the shape of its WxH size, else 16:9.
func videoAspect(req VideoRequest) string {
	w, h, ok := splitDims(req.AspectRatio, ":")
	if !ok {
		w, h, ok = videoDims(req.Size)
	}
	if !ok {
		return "16:9"
	}
	want := math.Log(float64(w) / float64(h))
	best, bestDiff := "16:9", math.Inf(1)
	for _, c := range []struct {
		name  string
		ratio float64
	}{{"16:9", 16.0 / 9}, {"9:16", 9.0 / 16}, {"1:1", 1}} {
		if d := math.Abs(want - math.Log(c.ratio)); d < bestDiff {
			best, bestDiff = c.name, d
		}
	}
	return best
}

// videoDims reads "1280x720" or Wan's own "1280*720".
func videoDims(size string) (int, int, bool) {
	if w, h, ok := splitDims(size, "x"); ok {
		return w, h, true
	}
	return splitDims(size, "*")
}

// nearestDuration clamps a requested length to the nearest one offered, ties
// going to the longer. Zero or less is the shortest.
func nearestDuration(seconds float64, offered []int) int {
	if seconds <= 0 {
		return offered[0]
	}
	best := offered[0]
	for _, d := range offered[1:] {
		if math.Abs(float64(d)-seconds) <= math.Abs(float64(best)-seconds) {
			best = d
		}
	}
	return best
}

type dashScopeTaskResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Output  struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		VideoURL   string `json:"video_url"`
		Code       string `json:"code"`
		Message    string `json:"message"`
	} `json:"output"`
	Usage struct {
		Duration            flexNumber `json:"duration"`
		OutputVideoDuration flexNumber `json:"output_video_duration"`
		SR                  flexNumber `json:"SR"`
	} `json:"usage"`
}

// flexNumber reads a JSON number that a vendor may also send as a string.
type flexNumber float64

func (f *flexNumber) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexNumber(v)
	return nil
}

// CreateVideo implements VideoAdapter. It is one attempt, never retried here:
// a repeated create after a lost answer would start, and bill, a second video.
func (a *DashScopeVideoAdapter) CreateVideo(ctx context.Context, req VideoRequest) (VideoJob, error) {
	body, model, tier, seconds := dashScopeVideoBody(req)
	payload, err := json.Marshal(body)
	if err != nil {
		return VideoJob{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/services/aigc/video-generation/video-synthesis", bytes.NewReader(payload))
	if err != nil {
		return VideoJob{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-DashScope-Async", "enable")
	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return VideoJob{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var parsed dashScopeTaskResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return VideoJob{}, fmt.Errorf("%s: invalid video response (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}
	if resp.StatusCode == http.StatusBadRequest {
		msg := strings.TrimSpace(parsed.Code + " " + parsed.Message)
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(req.ImageURL)), "data:") {
			msg += " (the first frame was sent inline as a data: URI; if the vendor will not take it, send an https:// URL instead)"
		}
		return VideoJob{}, fmt.Errorf("%w: %s refused the request: %s", ErrInvalidVideoRequest, a.name, msg)
	}
	if resp.StatusCode >= 400 || parsed.Code != "" {
		msg := parsed.Message
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return VideoJob{}, fmt.Errorf("%s: upstream error (status %d): %s %s", a.name, resp.StatusCode, parsed.Code, msg)
	}
	if parsed.Output.TaskID == "" {
		return VideoJob{}, fmt.Errorf("%s: no task id returned: %s", a.name, truncate(raw))
	}
	return VideoJob{TaskID: parsed.Output.TaskID, Model: model, Resolution: tier, Seconds: seconds}, nil
}

// VideoStatus implements VideoAdapter: one GET /tasks/{id}.
func (a *DashScopeVideoAdapter) VideoStatus(ctx context.Context, taskID string) (VideoStatus, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/tasks/"+url.PathEscape(taskID), nil)
	if err != nil {
		return VideoStatus{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return VideoStatus{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var parsed dashScopeTaskResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return VideoStatus{}, fmt.Errorf("%s: invalid task response (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}
	if resp.StatusCode >= 400 {
		return VideoStatus{}, fmt.Errorf("%s: task query failed (status %d): %s %s", a.name, resp.StatusCode, parsed.Code, parsed.Message)
	}
	return dashScopeVideoStatus(parsed), nil
}

// dashScopeVideoStatus maps a task onto the three states a caller sees.
// UNKNOWN is what DashScope says of a task it no longer has (they expire after
// a day), which is as final as a failure.
func dashScopeVideoStatus(t dashScopeTaskResponse) VideoStatus {
	switch strings.ToUpper(t.Output.TaskStatus) {
	case "PENDING", "RUNNING", "SUSPENDED":
		return VideoStatus{State: VideoProcessing}
	case "SUCCEEDED":
		if t.Output.VideoURL == "" {
			return VideoStatus{State: VideoFailed, Message: "the task succeeded without a video URL"}
		}
		st := VideoStatus{State: VideoSucceeded, URL: t.Output.VideoURL, Seconds: float64(t.Usage.OutputVideoDuration)}
		if st.Seconds <= 0 {
			st.Seconds = float64(t.Usage.Duration)
		}
		switch sr := float64(t.Usage.SR); {
		case sr >= 1080:
			st.Resolution = "1080P"
		case sr > 0:
			st.Resolution = "720P"
		}
		return st
	default:
		msg := strings.TrimSpace(t.Output.Code + " " + t.Output.Message)
		if msg == "" {
			msg = "video task " + strings.ToLower(firstNonEmpty(t.Output.TaskStatus, "unknown"))
		}
		return VideoStatus{State: VideoFailed, Message: msg}
	}
}
