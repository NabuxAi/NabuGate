package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiMediaPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inlineData,omitempty"`
}

type geminiMediaResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiMediaPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// generate posts a generateContent request and returns the parsed response.
func (a *GeminiAdapter) generate(ctx context.Context, model string, body any) (geminiMediaResponse, int, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return geminiMediaResponse{}, 0, err
	}
	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", a.baseURL, url.PathEscape(model), url.QueryEscape(a.apiKey))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return geminiMediaResponse{}, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return geminiMediaResponse{}, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var parsed geminiMediaResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return geminiMediaResponse{}, resp.StatusCode, fmt.Errorf("%s: invalid response (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}
	return parsed, resp.StatusCode, nil
}

// Image implements ImageAdapter. Gemini returns one image per call, so we loop
// to satisfy req.N.
func (a *GeminiAdapter) Image(ctx context.Context, req ImageRequest) (ImageResponse, error) {
	n := req.N
	if n <= 0 {
		n = 1
	}

	imageConfig := map[string]any{}
	if req.AspectRatio != "" {
		imageConfig["aspectRatio"] = req.AspectRatio
	}
	genCfg := map[string]any{"responseModalities": []string{"IMAGE"}}
	if len(imageConfig) > 0 {
		genCfg["imageConfig"] = imageConfig
	}

	out := ImageResponse{}
	for i := 0; i < n; i++ {
		body := map[string]any{
			"contents":         []any{map[string]any{"parts": []any{map[string]any{"text": req.Prompt}}}},
			"generationConfig": genCfg,
		}
		parsed, status, err := a.generate(ctx, req.Model, body)
		if err != nil {
			return ImageResponse{}, err
		}
		if status >= 400 {
			return ImageResponse{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, status, geminiErr(parsed.Error, status))
		}
		for _, c := range parsed.Candidates {
			for _, p := range c.Content.Parts {
				if p.InlineData != nil && p.InlineData.Data != "" {
					out.Images = append(out.Images, p.InlineData.Data)
				}
			}
		}
	}
	if len(out.Images) == 0 {
		return ImageResponse{}, fmt.Errorf("%s: no image returned", a.name)
	}
	return out, nil
}

// Speech implements SpeechAdapter. Gemini returns raw PCM (signed 16-bit LE);
// we wrap it in a WAV container so the gateway always returns a playable file.
func (a *GeminiAdapter) Speech(ctx context.Context, req SpeechRequest) (SpeechResponse, error) {
	voice := req.Voice
	if voice == "" {
		voice = "Kore"
	}
	body := map[string]any{
		"contents": []any{map[string]any{"parts": []any{map[string]any{"text": req.Input}}}},
		"generationConfig": map[string]any{
			"responseModalities": []string{"AUDIO"},
			"speechConfig": map[string]any{
				"voiceConfig": map[string]any{
					"prebuiltVoiceConfig": map[string]any{"voiceName": voice},
				},
			},
		},
	}

	parsed, status, err := a.generate(ctx, req.Model, body)
	if err != nil {
		return SpeechResponse{}, err
	}
	if status >= 400 {
		return SpeechResponse{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, status, geminiErr(parsed.Error, status))
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return SpeechResponse{}, fmt.Errorf("%s: no audio returned", a.name)
	}
	part := parsed.Candidates[0].Content.Parts[0]
	if part.InlineData == nil || part.InlineData.Data == "" {
		return SpeechResponse{}, fmt.Errorf("%s: no audio returned", a.name)
	}

	pcm, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("%s: bad audio data: %w", a.name, err)
	}
	rate := sampleRateFromMime(part.InlineData.MimeType)
	return SpeechResponse{Audio: wrapPCMAsWAV(pcm, rate), ContentType: "audio/wav"}, nil
}

func geminiErr(e *struct {
	Message string `json:"message"`
}, status int) string {
	if e != nil && e.Message != "" {
		return e.Message
	}
	return http.StatusText(status)
}

// sampleRateFromMime parses "audio/L16;codec=pcm;rate=24000" -> 24000.
func sampleRateFromMime(mime string) int {
	for _, part := range strings.Split(mime, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "rate=") {
			if r, err := strconv.Atoi(strings.TrimPrefix(part, "rate=")); err == nil && r > 0 {
				return r
			}
		}
	}
	return 24000
}

// wrapPCMAsWAV wraps mono 16-bit little-endian PCM in a WAV container.
func wrapPCMAsWAV(pcm []byte, sampleRate int) []byte {
	const (
		numChannels   = 1
		bitsPerSample = 16
	)
	byteRate := sampleRate * numChannels * bitsPerSample / 8
	blockAlign := numChannels * bitsPerSample / 8

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))               // PCM fmt chunk size
	binary.Write(&buf, binary.LittleEndian, uint16(1))                // audio format = PCM
	binary.Write(&buf, binary.LittleEndian, uint16(numChannels))      //
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))       //
	binary.Write(&buf, binary.LittleEndian, uint32(byteRate))         //
	binary.Write(&buf, binary.LittleEndian, uint16(blockAlign))       //
	binary.Write(&buf, binary.LittleEndian, uint16(bitsPerSample))    //
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)
	return buf.Bytes()
}
