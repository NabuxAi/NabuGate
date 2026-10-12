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

	// The Decide route has no adapter-side fallback: an upstream that answered
	// without a usage object would otherwise be metered at an indifferent
	// zero. Estimate from the payload actually sent — state and questions —
	// and the answers that came back, flagged as a guess.
	usage := result.Usage
	if usage.TotalTokens == 0 {
		pTok := provider.EstimateTokens(len(body.State) + len(body.Questions))
		cTok := provider.EstimateTokens(len(result.Answers))
		usage = provider.Usage{
			PromptTokens:     pTok,
			CompletionTokens: cTok,
			TotalTokens:      pTok + cTok,
			Estimated:        true,
		}
	}
	cost := s.record(r, result.Provider, result.Model, usage)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)
	// The charged amount rides the same header every other metered response
	// sets, so a caller reconciling their spend reads one place.
	w.Header().Set(costHeader, formatCost(cost))
	usageOut := map[string]any{
		"input_tokens":  usage.PromptTokens,
		"output_tokens": usage.CompletionTokens,
		"total_tokens":  usage.TotalTokens,
		// Additive: what the caller was charged, alongside the token counts.
		"cost_usd": cost,
	}
	if usage.Estimated {
		usageOut["estimated"] = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model":   result.Model,
		"answers": result.Answers,
		"usage":   usageOut,
	})
}
