package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"nabugate/internal/memory"
	"nabugate/internal/provider"
)

// Conversation memory.
//
// A client sends "conversation_id" alongside the usual OpenAI body. The gateway
// replays that conversation's history ahead of the new messages and stores the
// exchange afterwards, so an app gets memory without building a store.
//
// Entirely opt-in: a request without the field behaves exactly as before, byte
// for byte. That matters because every existing consumer is already sending
// requests through here.
//
// Scoping is the part to be careful with. A conversation belongs to one
// project, the project comes from the authenticated key rather than the body,
// and it is part of the storage path — so there is no request a client can
// construct that reads another project's history.

// conversationID extracts and validates the field. Returns "" when absent.
func conversationID(raw map[string]json.RawMessage) string {
	if len(raw["conversation_id"]) == 0 {
		return ""
	}
	var id string
	_ = json.Unmarshal(raw["conversation_id"], &id)
	return strings.TrimSpace(id)
}

// loadHistory prepends a conversation's stored turns to the request.
//
// History goes after any system message and before the new turns: a system
// prompt is an instruction about the whole exchange, and pushing it behind the
// history would let recalled text outrank it.
func (s *Server) loadHistory(r *http.Request, id string, req *provider.ChatRequest) (bool, error) {
	if s.memory == nil || id == "" {
		return false, nil
	}
	project := s.project(r)
	if project == "" {
		// An admin key has no project, so there is no conversation namespace to
		// use. Refusing beats silently writing into a shared one.
		return false, memory.ErrNoProject
	}

	conv, err := s.memory.Load(project, id)
	if err != nil {
		// A new id is not an error: this is how a conversation begins.
		return err == memory.ErrNotFound, nil
	}
	if len(conv.Messages) == 0 {
		return true, nil
	}

	var system, rest []provider.Message
	for _, m := range req.Messages {
		if m.Role == "system" {
			system = append(system, m)
		} else {
			rest = append(rest, m)
		}
	}

	history := make([]provider.Message, 0, len(conv.Messages))
	for _, m := range conv.Messages {
		history = append(history, provider.Message{Role: m.Role, Content: m.Content})
	}

	merged := make([]provider.Message, 0, len(system)+len(history)+len(rest))
	merged = append(merged, system...)
	merged = append(merged, history...)
	merged = append(merged, rest...)
	req.Messages = merged

	// The raw body is what OpenAI-wire adapters actually send, so it has to
	// carry the merged messages too — otherwise history would reach Anthropic
	// and Gemini but not OpenAI-compatible providers, which is most of them.
	if req.Raw != nil {
		if encoded, err := json.Marshal(merged); err == nil {
			req.Raw["messages"] = encoded
		}
		// Never forward our own field upstream: providers reject unknown keys.
		delete(req.Raw, "conversation_id")
	}
	return true, nil
}

// saveTurn stores the new user messages and the assistant's reply.
//
// Only the turns from this request are stored, not the replayed history, or a
// conversation would double in size every time it was used.
func (s *Server) saveTurn(r *http.Request, id string, newMsgs []provider.Message, reply string) {
	if s.memory == nil || id == "" {
		return
	}
	project := s.project(r)
	if project == "" {
		return
	}

	turns := make([]memory.Message, 0, len(newMsgs)+1)
	for _, m := range newMsgs {
		// System messages are instructions, not conversation: an agent sends
		// its own on every call, and storing them would replay a growing stack
		// of identical prompts.
		if m.Role == "system" {
			continue
		}
		turns = append(turns, memory.Message{Role: m.Role, Content: m.Content})
	}
	if strings.TrimSpace(reply) != "" {
		turns = append(turns, memory.Message{Role: "assistant", Content: reply})
	}
	if len(turns) == 0 {
		return
	}

	if _, err := s.memory.Append(project, id, turns...); err != nil {
		s.log.Warn("store conversation turn", "project", project, "conversation", id, "error", err)
	}
}

// ─────────────────────────── management API ─────────────────────────────────

// mountConversationAPI exposes a project's own conversations to that project.
func (s *Server) mountConversationAPI(mux *http.ServeMux) {
	if s.memory == nil {
		return
	}
	mux.HandleFunc("GET /v1/conversations", s.auth(s.listConversations))
	mux.HandleFunc("GET /v1/conversations/{id}", s.auth(s.getConversation))
	mux.HandleFunc("DELETE /v1/conversations/{id}", s.auth(s.deleteConversation))
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	project := s.project(r)
	if project == "" {
		writeError(w, http.StatusBadRequest, memory.ErrNoProject.Error())
		return
	}
	list, err := s.memory.List(project)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": list})
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request) {
	project := s.project(r)
	if project == "" {
		writeError(w, http.StatusBadRequest, memory.ErrNoProject.Error())
		return
	}
	conv, err := s.memory.Load(project, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	writeJSON(w, http.StatusOK, conv)
}

func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	project := s.project(r)
	if project == "" {
		writeError(w, http.StatusBadRequest, memory.ErrNoProject.Error())
		return
	}
	if err := s.memory.Delete(project, r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
