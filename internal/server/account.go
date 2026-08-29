package server

import "net/http"

// Per-user usage, for the panel at /panel/.
//
// The rest of that panel already exists — /api/me carries the balance and
// /api/tokens already narrows to the caller's own keys — so this adds the one
// question it could not answer: what have my keys actually spent.
//
// Scoped by the signed-in email and nothing else. The admin console answers the
// same question for the whole deployment; this one is only ever about the
// person asking.
func (s *Server) accountUsage(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	perProject := s.admin.UsageForOwner(email)

	var requests, prompt, completion, denied int64
	var cost float64
	for _, c := range perProject {
		requests += c.Requests
		prompt += c.PromptTokens
		completion += c.CompletionTokens
		denied += c.Denied
		cost += c.CostUSD
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"projects":          perProject,
		"requests":          requests,
		"prompt_tokens":     prompt,
		"completion_tokens": completion,
		"total_tokens":      prompt + completion,
		// Surfaced because a key being refused is what a customer most needs to
		// see, and the one thing an aggregate token count hides completely.
		"denied":   denied,
		"cost_usd": cost,
	})
}
