package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// DashScopeImageAdapter serves Alibaba's Wan text-to-image models (wan2.6-t2i
// and later) from Model Studio's native multimodal-generation API. The chat
// side of DashScope rides the openai adapter through its compatible-mode
// endpoint, but images have no OpenAI-wire route there: the request is a
// messages body under "input", and the answer is a signed OSS URL per image
// rather than base64. So this adapter posts that body synchronously, downloads
// each URL and returns the bytes base64-encoded like every other image rung.
type DashScopeImageAdapter struct {
	name    string
	baseURL string
	apiKey  string
}

// NewDashScopeImageAdapter builds the adapter. baseURL defaults to the
// international endpoint; the mainland one rejects international keys.
func NewDashScopeImageAdapter(name, baseURL, apiKey string) *DashScopeImageAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://dashscope-intl.aliyuncs.com/api/v1"
	}
	return &DashScopeImageAdapter{name: name, baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey}
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
	body := map[string]any{
		"model": req.Model,
		"input": map[string]any{"messages": []any{map[string]any{
			"role":    "user",
			"content": []any{map[string]any{"text": req.Prompt}},
		}}},
		"parameters": map[string]any{
			"size":      dashScopeSize(req),
			"n":         n,
			"watermark": false,
		},
	}
	if neg := strings.TrimSpace(req.NegativePrompt); neg != "" {
		body["parameters"].(map[string]any)["negative_prompt"] = neg
	}
	payload, err := json.Marshal(body)
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
	if s, ok := dashScopeSizes[strings.TrimSpace(req.AspectRatio)]; ok {
		return s
	}
	if w, h, ok := splitDims(req.Size, "x"); ok {
		return strconv.Itoa(w) + "*" + strconv.Itoa(h)
	}
	return dashScopeSizes["1:1"]
}
