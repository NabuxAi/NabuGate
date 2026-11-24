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
	"time"
)

// DashScopeImageAdapter serves Alibaba's Wan image models — text-to-image
// (wan2.6-t2i) and image editing with references (wan2.6-image) — from Model
// Studio's native multimodal-generation API. The chat
// side of DashScope rides the openai adapter through its compatible-mode
// endpoint, but images have no OpenAI-wire route there: the request is a
// messages body under "input", and the answer is a signed OSS URL per image
// rather than base64. So this adapter posts that body synchronously, downloads
// each URL and returns the bytes base64-encoded like every other image rung.
//
// The older Wan text-to-image models (wanText2ImageModels) are not on that
// API — it answers them "url error" — but on the asynchronous text2image one:
// a task is created and polled within the one call (see imageAsync).
type DashScopeImageAdapter struct {
	name    string
	baseURL string
	apiKey  string

	// pollEvery is the first wait between task polls, which then grows to
	// pollMax; pollTimeout bounds the whole wait. Fields so tests can shorten
	// them.
	pollEvery, pollMax, pollTimeout time.Duration
}

// NewDashScopeImageAdapter builds the adapter. baseURL defaults to the
// international endpoint; the mainland one rejects international keys.
func NewDashScopeImageAdapter(name, baseURL, apiKey string) *DashScopeImageAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://dashscope-intl.aliyuncs.com/api/v1"
	}
	return &DashScopeImageAdapter{
		name: name, baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey,
		pollEvery: 2 * time.Second, pollMax: 8 * time.Second, pollTimeout: 3 * time.Minute,
	}
}

func (a *DashScopeImageAdapter) Name() string { return a.name }

// Chat is unsupported: chat goes through the dashscope openai-wire provider.
func (a *DashScopeImageAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, fmt.Errorf("%s: chat is not supported by the DashScope image provider", a.name)
}

type dashScopeImageResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Output  struct {
		Choices []struct {
			Message struct {
				Content []struct {
					Image string `json:"image"`
				} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	} `json:"output"`
}

// Image implements ImageAdapter. n is sent explicitly: Wan defaults to four
// images a call, and each one is billed.
func (a *DashScopeImageAdapter) Image(ctx context.Context, req ImageRequest) (ImageResponse, error) {
	n := req.N
	if n <= 0 {
		n = 1
	}
	if spec, ok := wanText2ImageModels[strings.ToLower(req.Model)]; ok {
		return a.imageAsync(ctx, req, spec, n)
	}
	payload, err := json.Marshal(dashScopeImageBody(req, n))
	if err != nil {
		return ImageResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/services/aigc/multimodal-generation/generation", bytes.NewReader(payload))
	if err != nil {
		return ImageResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return ImageResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var parsed dashScopeImageResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ImageResponse{}, fmt.Errorf("%s: invalid image response (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}
	if resp.StatusCode >= 400 || parsed.Code != "" {
		msg := parsed.Message
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return ImageResponse{}, fmt.Errorf("%s: upstream error (status %d): %s %s", a.name, resp.StatusCode, parsed.Code, msg)
	}

	out := ImageResponse{}
	for _, c := range parsed.Output.Choices {
		for _, part := range c.Message.Content {
			if part.Image == "" || len(out.Images) >= n {
				continue
			}
			b64, err := downloadBase64(ctx, a.name, part.Image)
			if err != nil {
				return ImageResponse{}, err
			}
			out.Images = append(out.Images, b64)
		}
	}
	if len(out.Images) == 0 {
		return ImageResponse{}, fmt.Errorf("%s: no image returned", a.name)
	}
	return out, nil
}

// AcceptsInputImages implements ImageInputAdapter. Wan's image-in models
// (wan2.6-image) read input pictures from the same messages body; the -t2i
// models are text-to-image only, and an edit sent there would come back as a
// fresh picture that ignores the input.
func (a *DashScopeImageAdapter) AcceptsInputImages(model string) bool {
	m := strings.ToLower(model)
	if _, async := wanText2ImageModels[m]; async {
		return false
	}
	return !strings.Contains(m, "t2i")
}

// dashScopeImageBody is the multimodal-generation request. With input images
// the user turn is one {"image": …} part per picture followed by the prompt as
// {"text": …}, and size is sent only when the caller asked for a shape: left
// out, the model keeps the input's own shape, which is what an edit wants.
// Without input images the body is exactly the text-to-image one.
func dashScopeImageBody(req ImageRequest, n int) map[string]any {
	content := make([]any, 0, len(req.Images)+1)
	for _, img := range req.Images {
		content = append(content, map[string]any{"image": img})
	}
	content = append(content, map[string]any{"text": req.Prompt})

	params := map[string]any{
		"n":         n,
		"watermark": false,
	}
	if len(req.Images) == 0 {
		params["size"] = dashScopeSize(req)
	} else if size, ok := dashScopeRequestedSize(req); ok {
		params["size"] = size
	}
	if neg := strings.TrimSpace(req.NegativePrompt); neg != "" {
		params["negative_prompt"] = neg
	}
	return map[string]any{
		"model": req.Model,
		"input": map[string]any{"messages": []any{map[string]any{
			"role":    "user",
			"content": content,
		}}},
		"parameters": params,
	}
}

// dashScopeSizes are the canvases Wan is asked for per aspect ratio: each about
// 1.6 MP, inside the model's accepted pixel range.
var dashScopeSizes = map[string]string{
	"1:1":  "1280*1280",
	"16:9": "1696*960",
	"9:16": "960*1696",
	"4:3":  "1472*1104",
	"3:4":  "1104*1472",
	"3:2":  "1536*1024",
	"2:3":  "1024*1536",
	"4:5":  "1152*1440",
}

// dashScopeSize turns the request's aspect ratio, or else its OpenAI-style
// "WxH" size, into Wan's "W*H". Unknown shapes get the square default.
func dashScopeSize(req ImageRequest) string {
	if s, ok := dashScopeRequestedSize(req); ok {
		return s
	}
	return dashScopeSizes["1:1"]
}

// dashScopeRequestedSize is the canvas the caller asked for, if they asked for
// one Wan can draw: a known aspect ratio, else an OpenAI-style "WxH".
func dashScopeRequestedSize(req ImageRequest) (string, bool) {
	if s, ok := dashScopeSizes[strings.TrimSpace(req.AspectRatio)]; ok {
		return s, true
	}
	if w, h, ok := splitDims(req.Size, "x"); ok {
		return strconv.Itoa(w) + "*" + strconv.Itoa(h), true
	}
	return "", false
}

// --- The asynchronous text2image API (Wan 2.5 preview, Wan 2.2) ---

// wanText2Image is one asynchronous text-to-image model's canvas rules, from
// Model Studio's "Wan text-to-image V2 API reference" (international, read
// 2026-11-24):
// https://www.alibabacloud.com/help/en/model-studio/text-to-image-v2-api-reference
type wanText2Image struct {
	// Pixel-count bounds of the canvas (0: none).
	minArea, maxArea int
	// Bounds on each side (0: none).
	minSide, maxSide int
	// Bounds on width/height.
	minRatio, maxRatio float64
	// defaultSide is the square drawn when the caller asks for no shape, and
	// the area every other shape is drawn at.
	defaultSide int
	// maxN is the most pictures one task makes.
	maxN int
}

// wanText2ImageModels are the models served on the asynchronous text2image API
// rather than multimodal-generation. Text to image only: none reads an input
// picture.
var wanText2ImageModels = map[string]wanText2Image{
	// "Total pixels between [1280*1280, 1440*1440]; aspect ratio [1:4, 4:1]."
	"wan2.5-t2i-preview": {minArea: 1280 * 1280, maxArea: 1440 * 1440, minRatio: 0.25, maxRatio: 4, defaultSide: 1280, maxN: 4},
	// "Width and height between [512, 1440]"; default 1024*1024.
	"wan2.2-t2i-plus":  {minSide: 512, maxSide: 1440, minRatio: 512.0 / 1440, maxRatio: 1440.0 / 512, defaultSide: 1024, maxN: 4},
	"wan2.2-t2i-flash": {minSide: 512, maxSide: 1440, minRatio: 512.0 / 1440, maxRatio: 1440.0 / 512, defaultSide: 1024, maxN: 4},
}

// canvas is the size to ask the model for: the caller's shape (aspect ratio,
// else the shape of a WxH size) at the caller's pixel count if they gave a
// size, else at the model's default area — clamped into the model's bounds
// and rounded to multiples of 16. It never fails: a shape the model cannot
// draw becomes the nearest one it can.
func (m wanText2Image) canvas(req ImageRequest) string {
	side := float64(m.defaultSide)
	area := side * side
	ratio := 1.0
	if w, h, ok := splitDims(strings.TrimSpace(req.AspectRatio), ":"); ok {
		ratio = float64(w) / float64(h)
	} else if w, h, ok := splitDims(req.Size, "x"); ok {
		ratio = float64(w) / float64(h)
		area = float64(w) * float64(h)
	}
	ratio = math.Min(math.Max(ratio, m.minRatio), m.maxRatio)
	if m.minArea > 0 {
		area = math.Max(area, float64(m.minArea))
	}
	if m.maxArea > 0 {
		area = math.Min(area, float64(m.maxArea))
	}
	w, h := math.Sqrt(area*ratio), math.Sqrt(area/ratio)
	if m.maxSide > 0 {
		if k := float64(m.maxSide) / math.Max(w, h); k < 1 {
			w, h = w*k, h*k
		}
	}
	if m.minSide > 0 {
		if k := float64(m.minSide) / math.Min(w, h); k > 1 {
			w, h = w*k, h*k
		}
	}
	W, H := roundTo16(w), roundTo16(h)
	// Rounding may have stepped just outside a bound; step back in.
	for m.minArea > 0 && W*H < m.minArea {
		if W <= H {
			W += 16
		} else {
			H += 16
		}
	}
	for m.maxArea > 0 && W*H > m.maxArea {
		if W >= H {
			W -= 16
		} else {
			H -= 16
		}
	}
	W, H = clampSide(W, m.minSide, m.maxSide), clampSide(H, m.minSide, m.maxSide)
	return strconv.Itoa(W) + "*" + strconv.Itoa(H)
}

func roundTo16(v float64) int { return int(math.Round(v/16)) * 16 }

func clampSide(v, lo, hi int) int {
	if lo > 0 && v < lo {
		return lo
	}
	if hi > 0 && v > hi {
		return hi
	}
	return v
}

// dashScopeText2ImageBody is the text2image task: prompt and negative prompt
// under input, size and n under parameters, nothing else.
func dashScopeText2ImageBody(req ImageRequest, spec wanText2Image, n int) map[string]any {
	input := map[string]any{"prompt": req.Prompt}
	if neg := strings.TrimSpace(req.NegativePrompt); neg != "" {
		input["negative_prompt"] = neg
	}
	return map[string]any{
		"model":      req.Model,
		"input":      input,
		"parameters": map[string]any{"size": spec.canvas(req), "n": min(n, spec.maxN)},
	}
}

type dashScopeImageTask struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Output  struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		Code       string `json:"code"`
		Message    string `json:"message"`
		Results    []struct {
			URL     string `json:"url"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"results"`
	} `json:"output"`
}

// imageAsync creates a text2image task and polls it until it is done, then
// downloads each picture. The create is sent once and never retried: a
// repeated create after a lost answer would make, and bill, a second set. A
// poll that fails on the network or a 5xx is simply tried again until the
// deadline, since the task goes on regardless. A task still running at the
// deadline is reported as a failure; Alibaba may still finish it.
func (a *DashScopeImageAdapter) imageAsync(ctx context.Context, req ImageRequest, spec wanText2Image, n int) (ImageResponse, error) {
	if len(req.Images) > 0 {
		return ImageResponse{}, fmt.Errorf("%s: %s draws from text only and cannot take input images", a.name, req.Model)
	}
	payload, err := json.Marshal(dashScopeText2ImageBody(req, spec, n))
	if err != nil {
		return ImageResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/services/aigc/text2image/image-synthesis", bytes.NewReader(payload))
	if err != nil {
		return ImageResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-DashScope-Async", "enable")
	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return ImageResponse{}, err
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var created dashScopeImageTask
	if err := json.Unmarshal(raw, &created); err != nil {
		return ImageResponse{}, fmt.Errorf("%s: invalid image task response (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}
	if resp.StatusCode >= 400 || created.Code != "" {
		return ImageResponse{}, fmt.Errorf("%s: upstream error (status %d): %s %s", a.name, resp.StatusCode, created.Code, firstNonEmpty(created.Message, http.StatusText(resp.StatusCode)))
	}
	taskID := created.Output.TaskID
	if taskID == "" {
		return ImageResponse{}, fmt.Errorf("%s: no task id returned: %s", a.name, truncate(raw))
	}

	ctx, cancel := context.WithTimeout(ctx, a.pollTimeout)
	defer cancel()
	wait := a.pollEvery
	for {
		select {
		case <-ctx.Done():
			return ImageResponse{}, fmt.Errorf("%s: image task %s not finished after %s: %w", a.name, taskID, a.pollTimeout, ctx.Err())
		case <-time.After(wait):
		}
		wait = min(wait*3/2, a.pollMax)

		task, final, err := a.imageTask(ctx, taskID)
		if err != nil {
			return ImageResponse{}, err
		}
		if !final {
			continue
		}
		return a.imageTaskResult(ctx, task, n)
	}
}

// imageTask reads a task once. final is false while it is still running, and
// also when the poll itself failed in a way worth trying again.
func (a *DashScopeImageAdapter) imageTask(ctx context.Context, taskID string) (task dashScopeImageTask, final bool, err error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/tasks/"+url.PathEscape(taskID), nil)
	if err != nil {
		return task, false, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		// A network failure, or the deadline (which the loop reports).
		return task, false, nil
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if isTransient(resp.StatusCode) {
		return task, false, nil
	}
	if err := json.Unmarshal(raw, &task); err != nil {
		return task, false, fmt.Errorf("%s: invalid task response (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}
	if resp.StatusCode >= 400 {
		return task, false, fmt.Errorf("%s: task query failed (status %d): %s %s", a.name, resp.StatusCode, task.Code, task.Message)
	}
	switch strings.ToUpper(task.Output.TaskStatus) {
	case "PENDING", "RUNNING", "SUSPENDED":
		return task, false, nil
	}
	return task, true, nil
}

// imageTaskResult turns a finished task into pictures. A task can succeed with
// some results failed (each carrying its own code); those are skipped.
func (a *DashScopeImageAdapter) imageTaskResult(ctx context.Context, task dashScopeImageTask, n int) (ImageResponse, error) {
	if !strings.EqualFold(task.Output.TaskStatus, "SUCCEEDED") {
		msg := strings.TrimSpace(task.Output.Code + " " + task.Output.Message)
		if msg == "" {
			msg = "image task " + strings.ToLower(firstNonEmpty(task.Output.TaskStatus, "unknown"))
		}
		return ImageResponse{}, fmt.Errorf("%s: %s", a.name, msg)
	}
	out := ImageResponse{}
	why := ""
	for _, r := range task.Output.Results {
		if r.URL == "" {
			why = firstNonEmpty(why, strings.TrimSpace(r.Code+" "+r.Message))
			continue
		}
		if len(out.Images) >= n {
			break
		}
		b64, err := downloadBase64(ctx, a.name, r.URL)
		if err != nil {
			return ImageResponse{}, err
		}
		out.Images = append(out.Images, b64)
	}
	if len(out.Images) == 0 {
		return ImageResponse{}, fmt.Errorf("%s: no image returned %s", a.name, why)
	}
	return out, nil
}
