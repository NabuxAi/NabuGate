package server

import (
	"encoding/json"
	"net/http"

	"nabugate/internal/provider"
)

type decisionRequestBody struct {
	Model     string          `json:"model"`
	State     json.RawMessage `json:"state"`
	Questions json.RawMessage `json:"questions"`
}

func (s *Server) handleDecisions(w http.ResponseWriter, r *http.Request) {
	var body decisionRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Model == "" {
		writeError(w, http.StatusBadRequest, "field 'model' (alias) is required")
		return
	}
	if len(body.State) == 0 {
		writeError(w, http.StatusBadRequest, "field 'state' is required")
		return
	}
	if len(body.Questions) == 0 {
		writeError(w, http.StatusBadRequest, "field 'questions' is required")
		return
	}

	if !s.aliasAllowed(w, r, body.Model) {
		return
	}

	result, err := s.router.Decide(r.Context(), body.Model, provider.DecisionRequest{
		Model:     body.Model,
		State:     body.State,
		Questions: body.Questions,
	})
	if err != nil {
		writeError(w, aliasErrStatus(err, "unknown decision alias"), err.Error())
		return
	}

	s.record(r, result.Provider, result.Model, result.Usage)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)
	writeJSON(w, http.StatusOK, map[string]any{
		"model":   result.Model,
		"answers": result.Answers,
		"usage": map[string]any{
			"input_tokens":  result.Usage.PromptTokens,
			"output_tokens": result.Usage.CompletionTokens,
			"total_tokens":  result.Usage.TotalTokens,
		},
	})
}
