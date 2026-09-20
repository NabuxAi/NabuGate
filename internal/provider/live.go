package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// LiveSessionRequest is the body of a realtime voice session creation, carried
// opaquely. The gateway only rewrites the model: the session and transport
// objects (instructions, delegation, the browser's SDP offer) are the
// caller's contract with the vendor and change faster than this package.
type LiveSessionRequest struct {
	// Model is the upstream model the session runs on.
	Model string
	// Body is the caller's JSON body; "session.model" is replaced with Model.
	Body json.RawMessage
}

// LiveSessionResponse is the vendor's answer: the session id and the SDP
// answer the browser needs, returned verbatim so nothing the vendor adds
// later is lost on the way through.
type LiveSessionResponse struct {
	ID   string
	Body json.RawMessage
}

// LiveAdapter is implemented by providers that host a realtime, full-duplex
// voice session (OpenAI's GPT-Live). The audio never touches the gateway —
// the browser talks to the vendor over WebRTC — so the adapter's whole job
// is the signalling handshake.
type LiveAdapter interface {
	CreateLiveSession(ctx context.Context, req LiveSessionRequest) (LiveSessionResponse, error)
}

// LiveUpstreamError is the vendor refusing to open a live session. It keeps
// the vendor's status so the gateway can answer in the same class — a bad
// offer is the caller's to fix, a rejected key is ours — instead of a blanket
// 502 that reads the same for both.
type LiveUpstreamError struct {
	Status int
	// Message is what the vendor told its client: one line, anything shaped
	// like a credential masked. Safe to hand back to the caller.
	Message string
}

func (e *LiveUpstreamError) Error() string {
	return fmt.Sprintf("live session upstream %d: %s", e.Status, e.Message)
}

// maxLiveAnswerBytes bounds what is read of the vendor's answer. An SDP answer
// is a few kilobytes; anything near this is not one.
const maxLiveAnswerBytes = 1 << 20

// CreateLiveSession implements LiveAdapter over OpenAI's realtime calls API.
func (a *OpenAIAdapter) CreateLiveSession(ctx context.Context, req LiveSessionRequest) (LiveSessionResponse, error) {
	var incoming struct {
		Model     string          `json:"model"`
		Session   json.RawMessage `json:"session"`
		Transport struct {
			Type string `json:"type"`
			SDP  string `json:"sdp"`
		} `json:"transport"`
		SDP string `json:"sdp"`
	}
	_ = json.Unmarshal(req.Body, &incoming)

	sdp := incoming.Transport.SDP
	if sdp == "" {
		sdp = incoming.SDP
	}

	sessionMap := make(map[string]any)
	if len(incoming.Session) > 0 {
		_ = json.Unmarshal(incoming.Session, &sessionMap)
	}
	sessionMap["model"] = req.Model
	sessionJSON, _ := json.Marshal(sessionMap)

	// If an SDP offer is present, try OpenAI's GA WebRTC endpoint: POST /realtime/calls with multipart/form-data
	if sdp != "" {
		var formBody bytes.Buffer
		mw := multipart.NewWriter(&formBody)
		_ = mw.WriteField("sdp", sdp)
		_ = mw.WriteField("session", string(sessionJSON))
		_ = mw.Close()

		headers := a.headers()
		headers["Content-Type"] = mw.FormDataContentType()

		status, respHeader, raw, err := postRequestOnce(ctx, a.baseURL+"/realtime/calls", headers, formBody.Bytes())
		if err == nil && status >= 200 && status < 300 {
			callID := extractCallID(respHeader.Get("Location"), raw)
			sdpAnswer := string(raw)

			if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
				var parsed struct {
					ID        string `json:"id"`
					SDP       string `json:"sdp"`
					Session   struct {
						ID string `json:"id"`
					} `json:"session"`
					Transport struct {
						SDP string `json:"sdp"`
					} `json:"transport"`
				}
				if json.Unmarshal(raw, &parsed) == nil {
					if parsed.ID != "" {
						callID = parsed.ID
					} else if parsed.Session.ID != "" {
						callID = parsed.Session.ID
					}
					if parsed.Transport.SDP != "" {
						sdpAnswer = parsed.Transport.SDP
					} else if parsed.SDP != "" {
						sdpAnswer = parsed.SDP
					}
				}
			}

			respJSON, _ := json.Marshal(map[string]any{
				"id": callID,
				"session": map[string]any{
					"id": callID,
				},
				"transport": map[string]any{
					"type": "webrtc",
					"sdp":  sdpAnswer,
				},
			})
			return LiveSessionResponse{ID: callID, Body: respJSON}, nil
		}

		if err == nil && status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
			return LiveSessionResponse{}, &LiveUpstreamError{Status: status, Message: vendorErrorMessage(raw)}
		}
	}

	// Fallback path: legacy JSON POST to /realtime/sessions (used by test mocks or older proxies)
	body, err := rewriteLiveModel(req.Body, req.Model)
	if err != nil {
		return LiveSessionResponse{}, err
	}
	status, _, raw, err := postRequestOnce(ctx, a.baseURL+"/realtime/sessions", a.headers(), body)
	if err != nil {
		return LiveSessionResponse{}, fmt.Errorf("live session request failed: %w", err)
	}
	if status < 200 || status >= 300 {
		return LiveSessionResponse{}, &LiveUpstreamError{Status: status, Message: vendorErrorMessage(raw)}
	}
	var parsed struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &parsed)
	id := parsed.Session.ID
	if id == "" {
		id = parsed.ID
	}
	if id == "" {
		return LiveSessionResponse{}, fmt.Errorf("live session upstream returned no session id")
	}
	return LiveSessionResponse{ID: id, Body: raw}, nil
}

func extractCallID(location string, raw []byte) string {
	if location != "" {
		parts := strings.Split(strings.Trim(location, "/"), "/")
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			return parts[len(parts)-1]
		}
	}
	var parsed struct {
		ID      string `json:"id"`
		CallID  string `json:"call_id"`
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if json.Unmarshal(raw, &parsed) == nil {
		if parsed.CallID != "" {
			return parsed.CallID
		}
		if parsed.Session.ID != "" {
			return parsed.Session.ID
		}
		if parsed.ID != "" {
			return parsed.ID
		}
	}
	return fmt.Sprintf("call_%d", time.Now().UnixNano())
}

// postRequestOnce is an HTTP POST without retries, returning status, headers and body.
func postRequestOnce(ctx context.Context, url string, headers map[string]string, body []byte) (int, http.Header, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxLiveAnswerBytes))
	if err != nil {
		return resp.StatusCode, resp.Header, nil, err
	}
	return resp.StatusCode, resp.Header, raw, nil
}

// postJSONOnce is postJSON without the retries, for callers expecting (int, []byte, error).
func postJSONOnce(ctx context.Context, url string, headers map[string]string, body []byte) (int, []byte, error) {
	status, _, raw, err := postRequestOnce(ctx, url, headers, body)
	return status, raw, err
}

// credentialShaped matches what in an error body could be a secret: an
// "sk-"-style key or a bearer token. Vendors mask their own echo of a rejected
// key, but a proxy in between may not.
var credentialShaped = regexp.MustCompile(`(?i)(sk-[a-z0-9_\-]{6,}|bearer\s+[a-z0-9._\-]{8,})`)

// vendorErrorMessage is what the caller may read of a vendor's refusal: the
// message the vendor wrote for its client when the body carries one, a short
// excerpt otherwise — on one line, with anything credential-shaped masked.
func vendorErrorMessage(raw []byte) string {
	var parsed struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	msg := ""
	if json.Unmarshal(raw, &parsed) == nil {
		var inner struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(parsed.Error, &inner) == nil && inner.Message != "" {
			msg = inner.Message
		} else if s := ""; json.Unmarshal(parsed.Error, &s) == nil && s != "" {
			msg = s
		} else {
			msg = parsed.Message
		}
	}
	if msg == "" {
		msg = string(raw)
	}
	msg = strings.Join(strings.Fields(msg), " ")
	msg = credentialShaped.ReplaceAllString(msg, "[redacted]")
	if msg == "" {
		return "(empty response)"
	}
	return truncateBody([]byte(msg), 300)
}

// rewriteLiveModel sets session.model on the caller's body. The alias the
// caller named is the gateway's, not the vendor's; the vendor sees only the
// upstream model. A body with no "session" object gets one.
func rewriteLiveModel(body json.RawMessage, model string) ([]byte, error) {
	var top map[string]json.RawMessage
	if len(bytes.TrimSpace(body)) == 0 {
		top = map[string]json.RawMessage{}
	} else if err := json.Unmarshal(body, &top); err != nil {
		return nil, fmt.Errorf("live session body is not a JSON object: %w", err)
	}
	session := map[string]json.RawMessage{}
	if raw, ok := top["session"]; ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &session); err != nil {
			return nil, fmt.Errorf("live session \"session\" is not a JSON object: %w", err)
		}
	}
	modelJSON, _ := json.Marshal(model)
	session["model"] = modelJSON
	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	top["session"] = sessionJSON
	// The alias rides at the top level on the way in, mirroring every other
	// endpoint; it is not part of the vendor's contract.
	delete(top, "model")
	return json.Marshal(top)
}

func truncateBody(raw []byte, max int) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
