package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"nabugate/internal/policy"
	"nabugate/internal/preset"
	"nabugate/internal/provider"
)

// skillsHeader lets a caller switch skills on without touching the body, and
// echoes back which skills a response was produced with.
const skillsHeader = "X-Nabu-Skills"

// WithPresets attaches the skill and template registry.
func (s *Server) WithPresets(p *preset.Registry) *Server {
	s.presets = p
	return s
}

// requestSkills resolves the skills a chat request asked for, from the
// "nabu_skills" body field and the X-Nabu-Skills header.
func (s *Server) requestSkills(raw map[string]json.RawMessage, header string) ([]preset.Skill, error) {
	names, err := preset.ParseSkillNames(raw["nabu_skills"], header)
	if err != nil || len(names) == 0 {
		return nil, err
	}
	return s.presets.ResolveSkills(names)
}

func skillNames(skills []preset.Skill) string {
	out := make([]string, len(skills))
	for i, sk := range skills {
		out[i] = sk.Name
	}
	return strings.Join(out, ",")
}

// applySkillsToChat adds the skills' instructions as one system message placed
// after any leading system messages (an agent's prompt, the caller's own), so
// the caller's own system prompt is read first and the skill refines it. A
// skill's response_format and temperature apply only when the caller did not
// set them.
func applySkillsToChat(skills []preset.Skill, req *provider.ChatRequest) {
	text := preset.SystemText(skills)
	if text == "" {
		return
	}
	sysMsg := provider.Message{Role: "system", Content: text}
	at := 0
	for at < len(req.Messages) && req.Messages[at].Role == "system" {
		at++
	}
	req.Messages = append(req.Messages[:at:at], append([]provider.Message{sysMsg}, req.Messages[at:]...)...)
	if req.Raw != nil {
		var msgs []json.RawMessage
		_ = json.Unmarshal(req.Raw["messages"], &msgs)
		rawAt := 0
		for rawAt < len(msgs) {
			var head struct {
				Role string `json:"role"`
			}
			_ = json.Unmarshal(msgs[rawAt], &head)
			if head.Role != "system" && head.Role != "developer" {
				break
			}
			rawAt++
		}
		sysRaw, _ := json.Marshal(sysMsg)
		merged := append(append(append([]json.RawMessage{}, msgs[:rawAt]...), sysRaw), msgs[rawAt:]...)
		req.Raw["messages"], _ = json.Marshal(merged)
	}
	for _, sk := range skills {
		if sk.Temperature != nil && req.Temperature == nil {
			t := *sk.Temperature
			req.Temperature = &t
			setRawParam(req.Raw, "temperature", t)
		}
		if sk.ResponseFormat != "" {
			setRawParam(req.Raw, "response_format", map[string]string{"type": sk.ResponseFormat})
		}
	}
}

// handleSkills lists the skills (GET /v1/skills).
func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	data := s.presets.Skills()
	if data == nil {
		data = []preset.Skill{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// handleTemplates lists the templates (GET /v1/templates). A scoped key only
// sees templates whose model it may call.
func (s *Server) handleTemplates(w http.ResponseWriter, r *http.Request) {
	data := make([]preset.Template, 0)
	for _, t := range s.presets.Templates() {
		if s.callerMayUse(r, t.Model) {
			data = append(data, t)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// handleTemplate returns one template (GET /v1/templates/{name}).
func (s *Server) handleTemplate(w http.ResponseWriter, r *http.Request) {
	t, ok := s.presets.Template(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown template")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

type templateRunRequest struct {
	Variables   map[string]string `json:"variables"`
	Model       string            `json:"model"`
	Skills      []string          `json:"skills"`
	Stream      bool              `json:"stream"`
	Temperature *float64          `json:"temperature"`
	MaxTokens   *int              `json:"max_tokens"`
	// ConversationID carries on a conversation, as on /v1/chat/completions.
	ConversationID string `json:"conversation_id"`
}

// buildTemplateChat turns a template run into the chat body it stands for.
func buildTemplateChat(t preset.Template, in templateRunRequest) (map[string]any, error) {
	system, user, err := t.Render(in.Variables)
	if err != nil {
		return nil, err
	}
	msgs := []map[string]any{}
	if system != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": system})
	}
	msgs = append(msgs, map[string]any{"role": "user", "content": user})
	model := strings.TrimSpace(in.Model)
	if model == "" {
		model = t.Model
	}
	body := map[string]any{"model": model, "messages": msgs}
	skills := append(append([]string{}, t.Skills...), in.Skills...)
	if len(skills) > 0 {
		body["nabu_skills"] = skills
	}
	if in.Temperature != nil {
		body["temperature"] = *in.Temperature
	} else if t.Temperature != nil {
		body["temperature"] = *t.Temperature
	}
	if in.MaxTokens != nil {
		body["max_tokens"] = *in.MaxTokens
	} else if t.MaxTokens != nil {
		body["max_tokens"] = *t.MaxTokens
	}
	if in.Stream {
		body["stream"] = true
	}
	if in.ConversationID != "" {
		body["conversation_id"] = in.ConversationID
	}
	return body, nil
}

// handleTemplateRun runs a template (POST /v1/templates/{name}/run). It is a
// thin front for /v1/chat/completions: the filled-in request goes through the
// same handler, so allow-lists, rate limits, billing, fallbacks, streaming and
// the response shape are exactly those of a chat completion.
func (s *Server) handleTemplateRun(w http.ResponseWriter, r *http.Request) {
	t, ok := s.presets.Template(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown template (GET /v1/templates lists them)")
		return
	}
	var in templateRunRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body, err := buildTemplateChat(t, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload, _ := json.Marshal(body)
	r2 := r.Clone(r.Context())
	r2.Body = io.NopCloser(bytes.NewReader(payload))
	r2.ContentLength = int64(len(payload))
	w.Header().Set("X-Nabu-Template", t.Name)
	s.handleChat(w, r2)
}

// callerMayUse reports whether the request's key may call model, without
// writing an error — for filtering lists.
func (s *Server) callerMayUse(r *http.Request, model string) bool {
	if !s.authEnforced() {
		return true
	}
	pol, ok := r.Context().Value(policyCtxKey{}).(policy.Policy)
	return ok && pol.Allows(model)
}

// listPresets feeds the console's Templates screen (GET /api/presets).
func (s *Server) listPresets(w http.ResponseWriter, r *http.Request) {
	skills := s.presets.Skills()
	if skills == nil {
		skills = []preset.Skill{}
	}
	templates := s.presets.Templates()
	if templates == nil {
		templates = []preset.Template{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": skills, "templates": templates})
}

// testTemplate previews a template from the console (admin only, like
// testAgent): it renders the variables, applies the skills and returns the
// reply, without a gateway token.
func (s *Server) testTemplate(w http.ResponseWriter, r *http.Request) {
	t, ok := s.presets.Template(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown template")
		return
	}
	var in templateRunRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body, err := buildTemplateChat(t, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload, _ := json.Marshal(body)
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(payload, &raw)
	skills, err := s.requestSkills(raw, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	delete(raw, "nabu_skills")
	delete(raw, "stream")
	var msgs []provider.Message
	_ = json.Unmarshal(raw["messages"], &msgs)
	req := provider.ChatRequest{Messages: msgs, Raw: raw}
	if len(raw["temperature"]) > 0 {
		_ = json.Unmarshal(raw["temperature"], &req.Temperature)
	}
	if len(raw["max_tokens"]) > 0 {
		_ = json.Unmarshal(raw["max_tokens"], &req.MaxTokens)
	}
	applySkillsToChat(skills, &req)
	model, _ := body["model"].(string)
	result, err := s.router.Chat(r.Context(), model, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"content":  result.Response.Content,
		"provider": result.Provider,
		"model":    result.Model,
	})
}
