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

// DashScopeVideoAdapter serves Alibaba's Wan video models from Model Studio's
// native, asynchronous video-synthesis API: CreateVideo submits a task with
// X-DashScope-Async and returns its id, and VideoStatus reads GET /tasks/{id}.
// A finished task carries a signed OSS URL (valid about 24 hours) and the
// duration and tier the vendor bills for.
//
// Each model takes its own sizes, tiers and lengths (wanVideoModels), and the
// request is mapped onto the nearest of them rather than refused. An alias
// names its text-to-video model; a request with a first frame runs on the
// image-to-video model the alias names beside it (VideoRequest.ImageModel).
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

// wanVideoModel is what one Wan video model accepts. Read from Model Studio's
// international API references on 2026-11-24 — "Wan - text-to-video API
// reference (2.1-2.6)" and the image-to-video (first frame) reference:
// https://www.alibabacloud.com/help/en/model-studio/legacy-wan-text-to-video-api-reference
// https://www.alibabacloud.com/help/en/model-studio/image-to-video-api-reference
type wanVideoModel struct {
	// imageToVideo models take a first frame (input.img_url) and a tier
	// (parameters.resolution); the output follows the frame's shape.
	// Text-to-video models take a canvas (parameters.size) instead.
	imageToVideo bool
	// tiers are the resolutions offered, lowest first.
	tiers []string
	// durations are the lengths offered, in seconds. A single one is the
	// model's fixed length, which it does not let a caller set, so duration
	// is not sent at all.
	durations []int
}

// wanVideoModels is every Wan video model this key can run (2026-11-24: the
// create was accepted for each). wanx2.1-kf2v-plus, wanx2.1-vace-plus and
// wan2.2-s2v are not on the key and are not here.
var wanVideoModels = map[string]wanVideoModel{
	"wan2.6-t2v":         {tiers: []string{"720P", "1080P"}, durations: secondsFrom(2, 15)},
	"wan2.6-i2v":         {imageToVideo: true, tiers: []string{"720P", "1080P"}, durations: secondsFrom(2, 15)},
	"wan2.5-t2v-preview": {tiers: []string{"480P", "720P", "1080P"}, durations: []int{5, 10}},
	"wan2.5-i2v-preview": {imageToVideo: true, tiers: []string{"480P", "720P", "1080P"}, durations: []int{5, 10}},
	"wan2.2-t2v-plus":    {tiers: []string{"480P", "1080P"}, durations: []int{5}},
	"wan2.2-i2v-plus":    {imageToVideo: true, tiers: []string{"480P", "1080P"}, durations: []int{5}},
	"wan2.2-i2v-flash":   {imageToVideo: true, tiers: []string{"480P", "720P"}, durations: []int{5}},
	"wan2.1-t2v-turbo":   {tiers: []string{"480P", "720P"}, durations: []int{5}},
	"wan2.1-t2v-plus":    {tiers: []string{"720P"}, durations: []int{5}},
	"wan2.1-i2v-turbo":   {imageToVideo: true, tiers: []string{"480P", "720P"}, durations: []int{3, 4, 5}},
	"wan2.1-i2v-plus":    {imageToVideo: true, tiers: []string{"720P"}, durations: []int{5}},
}

// WanVideoModel reports whether model is a Wan video model this adapter knows,
// whether it makes video from a first frame, and the tiers it renders. The
// config tests use it to check that every alias names models the adapter can
// map, and that every tier each one renders has a price.
func WanVideoModel(model string) (imageToVideo bool, tiers []string, ok bool) {
	spec, ok := wanVideoModels[model]
	return spec.imageToVideo, append([]string(nil), spec.tiers...), ok
}

func secondsFrom(lo, hi int) []int {
	out := make([]int, 0, hi-lo+1)
	for s := lo; s <= hi; s++ {
		out = append(out, s)
	}
	return out
}

// wanVideoCanvases are the text-to-video sizes per tier and shape. 480P has no
// 4:3 or 3:4.
var wanVideoCanvases = map[string]map[string]string{
	"480P":  {"16:9": "832*480", "9:16": "480*832", "1:1": "624*624"},
	"720P":  {"16:9": "1280*720", "9:16": "720*1280", "1:1": "960*960", "4:3": "1088*832", "3:4": "832*1088"},
	"1080P": {"16:9": "1920*1080", "9:16": "1080*1920", "1:1": "1440*1440", "4:3": "1632*1248", "3:4": "1248*1632"},
}

// wanTierHeights are each tier's short side, which is what "nearest tier"
// compares.
var wanTierHeights = map[string]float64{"480P": 480, "720P": 720, "1080P": 1080}

// wanDefaultTier and wanDefaultSeconds are what a request that asks for
// neither gets, before the model's own offer is applied: 720P and 5 s.
const (
	wanDefaultTier    = "720P"
	wanDefaultSeconds = 5
)

// dashScopeVideoBody maps a request onto Wan's: the model (the alias's
// image-to-video one when there is a first frame), the input and the
// parameters, plus the tier and duration chosen. Only parameters the chosen
// model accepts are sent.
func dashScopeVideoBody(req VideoRequest) (body map[string]any, model, tier string, seconds int, err error) {
	model = req.Model
	img := strings.TrimSpace(req.ImageURL)
	if img != "" {
		if m := strings.TrimSpace(req.ImageModel); m != "" {
			model = m
		} else if spec, ok := wanVideoModels[model]; ok && !spec.imageToVideo {
			return nil, "", "", 0, fmt.Errorf("%w: %s makes video from text only, and this alias names no image-to-video model; send the request without image_url or use an alias that animates a photo", ErrInvalidVideoRequest, model)
		}
	}
	spec, ok := wanVideoModels[model]
	if !ok {
		// A configuration fault, not the caller's: no request is sent with
		// parameters nobody checked against the model.
		return nil, "", "", 0, fmt.Errorf("video model %q is not in the DashScope adapter's model table", model)
	}
	if img == "" && spec.imageToVideo {
		return nil, "", "", 0, fmt.Errorf("%w: %s animates a first frame; send image_url", ErrInvalidVideoRequest, model)
	}

	tier = nearestTier(requestedTier(req), spec.tiers)
	seconds = nearestDuration(req.Seconds, spec.durations)

	input := map[string]any{}
	if p := strings.TrimSpace(req.Prompt); p != "" {
		input["prompt"] = p
	}
	if neg := strings.TrimSpace(req.NegativePrompt); neg != "" {
		input["negative_prompt"] = neg
	}
	params := map[string]any{}
	if len(spec.durations) > 1 {
		params["duration"] = seconds
	}
	if spec.imageToVideo {
		// Image-to-video follows the first frame's shape; only the tier is
		// chosen.
		input["img_url"] = img
		params["resolution"] = tier
	} else {
		params["size"] = wanVideoCanvases[tier][videoAspect(req, wanVideoCanvases[tier])]
	}
	return map[string]any{"model": model, "input": input, "parameters": params}, model, tier, seconds, nil
}

// requestedTier is the tier the caller asked for: an explicit resolution, else
// a tier written as the size ("1080p"), else the tier whose canvas is nearest
// a WxH size's pixel count, else the default.
func requestedTier(req VideoRequest) string {
	for _, v := range []string{req.Resolution, req.Size} {
		switch strings.ToUpper(strings.TrimSpace(v)) {
		case "1080P", "1080":
			return "1080P"
		case "720P", "720":
			return "720P"
		case "480P", "480":
			return "480P"
		}
	}
	if w, h, ok := videoDims(req.Size); ok {
		return tierOfDims(w, h)
	}
	return wanDefaultTier
}

// tierOfDims is the tier a WxH canvas belongs to: the one whose 16:9 canvas
// has the nearest pixel count. 1440*1440 and 1632*1248 are 1080P, 960*960
// and 1088*832 are 720P, 624*624 is 480P.
func tierOfDims(w, h int) string {
	area := math.Log(float64(w) * float64(h))
	best, bestDiff := wanDefaultTier, math.Inf(1)
	for _, t := range []string{"480P", "720P", "1080P"} {
		ht := wanTierHeights[t]
		if d := math.Abs(area - math.Log(ht*ht*16/9)); d < bestDiff {
			best, bestDiff = t, d
		}
	}
	return best
}

// nearestTier is the offered tier nearest the one asked for, on a log scale of
// the short side, ties going to the higher: 720P on a model that offers only
// 480P and 1080P (720 is their geometric mean) renders at 1080P.
func nearestTier(want string, offered []string) string {
	wh, ok := wanTierHeights[want]
	if !ok {
		wh = wanTierHeights[wanDefaultTier]
	}
	best, bestDiff := offered[0], math.Inf(1)
	for _, t := range offered {
		if d := math.Abs(math.Log(wanTierHeights[t] / wh)); d <= bestDiff+1e-12 {
			best, bestDiff = t, d
		}
	}
	return best
}

// videoAspect picks the canvas shape nearest the request's aspect ratio, else
// the shape of its WxH size, else 16:9, among the shapes a tier offers.
func videoAspect(req VideoRequest, canvases map[string]string) string {
	w, h, ok := splitDims(req.AspectRatio, ":")
	if !ok {
		w, h, ok = videoDims(req.Size)
	}
	if !ok {
		return "16:9"
	}
	want := math.Log(float64(w) / float64(h))
	best, bestDiff := "16:9", math.Inf(1)
	for _, name := range []string{"16:9", "9:16", "1:1", "4:3", "3:4"} {
		if _, offered := canvases[name]; !offered {
			continue
		}
		cw, ch, _ := splitDims(name, ":")
		if d := math.Abs(want - math.Log(float64(cw)/float64(ch))); d < bestDiff {
			best, bestDiff = name, d
		}
	}
	return best
}

// videoDims reads "1280x720", Wan's own "1280*720", or "1280_720".
func videoDims(size string) (int, int, bool) {
	for _, sep := range []string{"x", "*", "_"} {
		if w, h, ok := splitDims(size, sep); ok {
			return w, h, true
		}
	}
	return 0, 0, false
}

// nearestDuration clamps a requested length to the nearest one offered, ties
// going to the longer. Zero or less asks for the default 5 s.
func nearestDuration(seconds float64, offered []int) int {
	if seconds <= 0 {
		seconds = wanDefaultSeconds
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
	// Usage is spelled two ways. Wan 2.6 reports duration (input plus
	// output), output_video_duration, SR (the tier, e.g. 720) and size; Wan 2.5
	// and earlier report video_duration and video_ratio ("832*480").
	Usage struct {
		Duration            flexNumber `json:"duration"`
		OutputVideoDuration flexNumber `json:"output_video_duration"`
		SR                  flexNumber `json:"SR"`
		Size                string     `json:"size"`
		VideoDuration       flexNumber `json:"video_duration"`
		VideoRatio          string     `json:"video_ratio"`
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
	body, model, tier, seconds, err := dashScopeVideoBody(req)
	if err != nil {
		return VideoJob{}, err
	}
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
		st := VideoStatus{State: VideoSucceeded, URL: t.Output.VideoURL}
		for _, d := range []flexNumber{t.Usage.OutputVideoDuration, t.Usage.Duration, t.Usage.VideoDuration} {
			if d > 0 {
				st.Seconds = float64(d)
				break
			}
		}
		switch sr := float64(t.Usage.SR); {
		case sr >= 1080:
			st.Resolution = "1080P"
		case sr >= 720:
			st.Resolution = "720P"
		case sr > 0:
			st.Resolution = "480P"
		default:
			for _, dims := range []string{t.Usage.Size, t.Usage.VideoRatio} {
				if w, h, ok := videoDims(dims); ok {
					st.Resolution = tierOfDims(w, h)
					break
				}
			}
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
