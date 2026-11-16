package provider

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/url"
	"strings"
)

// buildGeminiContents turns an OpenAI chat into Gemini contents.
//
// Text messages stay text. An image_url part is not text: a data URL becomes
// inlineData (the base64 payload, without the data: prefix) and an https URL
// becomes fileData.fileUri so Gemini fetches the picture itself. The gateway
// does not download it. http URLs are refused; a cleartext image is not a
// screenshot we will ask Google to fetch.
func buildGeminiContents(req ChatRequest) (*geminiContent, []geminiContent, error) {
	if raw, ok := req.Raw["messages"]; ok && len(raw) > 0 {
		return contentsFromRaw(raw)
	}
	var system *geminiContent
	contents := make([]geminiContent, 0, len(req.Messages))
	for _, m := range req.Messages {
		parts := []any{geminiPart{Text: m.Content}}
		switch m.Role {
		case "system":
			if system == nil {
				system = &geminiContent{Parts: parts}
			} else {
				system.Parts = append(system.Parts, parts...)
			}
		case "assistant":
			contents = append(contents, geminiContent{Role: "model", Parts: parts})
		default:
			contents = append(contents, geminiContent{Role: "user", Parts: parts})
		}
	}
	return system, contents, nil
}

func contentsFromRaw(raw json.RawMessage) (*geminiContent, []geminiContent, error) {
	var msgs []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil, nil, fmt.Errorf("gemini: messages: %w", err)
	}
	var system *geminiContent
	contents := make([]geminiContent, 0, len(msgs))
	for i, m := range msgs {
		parts, err := openAIContentToGemini(m.Content)
		if err != nil {
			return nil, nil, fmt.Errorf("gemini: messages[%d]: %w", i, err)
		}
		if len(parts) == 0 {
			continue
		}
		switch m.Role {
		case "system":
			if system == nil {
				system = &geminiContent{Parts: parts}
			} else {
				system.Parts = append(system.Parts, parts...)
			}
		case "assistant":
			contents = append(contents, geminiContent{Role: "model", Parts: parts})
		default:
			contents = append(contents, geminiContent{Role: "user", Parts: parts})
		}
	}
	return system, contents, nil
}

func openAIContentToGemini(raw json.RawMessage) ([]any, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, err
		}
		if text == "" {
			return nil, nil
		}
		return []any{geminiPart{Text: text}}, nil
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("content is not text or an array of parts: %w", err)
	}
	out := make([]any, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "", "text":
			if p.Text != "" {
				out = append(out, geminiPart{Text: p.Text})
			}
		case "image_url":
			part, err := geminiImagePart(p.ImageURL.URL)
			if err != nil {
				return nil, err
			}
			out = append(out, part)
		default:
			if p.Text != "" {
				out = append(out, geminiPart{Text: p.Text})
			}
		}
	}
	return out, nil
}

func geminiImagePart(rawURL string) (any, error) {
	rawURL = strings.TrimSpace(rawURL)
	if strings.HasPrefix(rawURL, "data:") {
		mimeType, data, ok := splitDataURL(rawURL)
		if !ok {
			return nil, fmt.Errorf("image data URL is not base64")
		}
		if !strings.HasPrefix(mimeType, "image/") {
			return nil, fmt.Errorf("image data URL is %s, not an image", mimeType)
		}
		return map[string]any{
			"inlineData": map[string]string{"mimeType": mimeType, "data": data},
		}, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("image URL must be a data: URL or https")
	}
	return map[string]any{
		"fileData": map[string]string{
			"mimeType": mimeFromURL(u),
			"fileUri":  rawURL,
		},
	}, nil
}

func splitDataURL(v string) (mimeType, data string, ok bool) {
	rest, found := strings.CutPrefix(v, "data:")
	if !found {
		return "", "", false
	}
	meta, payload, found := strings.Cut(rest, ",")
	if !found || !strings.Contains(meta, ";base64") || payload == "" {
		return "", "", false
	}
	mimeType = strings.TrimSpace(strings.Split(meta, ";")[0])
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return mimeType, payload, true
}

func mimeFromURL(u *url.URL) string {
	ext := ""
	if i := strings.LastIndex(u.Path, "."); i >= 0 {
		ext = u.Path[i:]
	}
	if t := mime.TypeByExtension(ext); strings.HasPrefix(t, "image/") {
		return t
	}
	return "image/jpeg"
}
