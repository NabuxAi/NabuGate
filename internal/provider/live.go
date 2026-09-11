package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

// CreateLiveSession implements LiveAdapter over OpenAI's POST /live/sessions.
func (a *OpenAIAdapter) CreateLiveSession(ctx context.Context, req LiveSessionRequest) (LiveSessionResponse, error) {
	body, err := rewriteLiveModel(req.Body, req.Model)
	if err != nil {
		return LiveSessionResponse{}, err
	}
	// The offer is a fixed body, so replaying it on a transient failure is
	// safe: the browser has not applied any answer yet.
	status, raw, err := postJSON(ctx, a.baseURL+"/live/sessions", a.headers(), body, a.name+" live session")
	if err != nil {
		return LiveSessionResponse{}, fmt.Errorf("live session request failed: %w", err)
	}
	if status < 200 || status >= 300 {
		return LiveSessionResponse{}, fmt.Errorf("live session upstream %d: %s", status, truncateBody(raw, 300))
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
