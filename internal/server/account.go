package server

import (
	"net/http"
	"sort"
	"strings"

	"nabugate/internal/adminstore"
)

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
	var cost, providerCost float64
	for _, c := range perProject {
		requests += c.Requests
		prompt += c.PromptTokens
		completion += c.CompletionTokens
		denied += c.Denied
		cost += c.CostUSD
		providerCost += c.ProviderCostUSD
	}
	// Where the credit went: per model, most expensive first, and per day for
	// the last thirty.
	byModel, days := s.admin.OwnerBreakdown(email, 30)
	type modelRow struct {
		Model            string  `json:"model"`
		Requests         int64   `json:"requests"`
		PromptTokens     int64   `json:"prompt_tokens"`
		CompletionTokens int64   `json:"completion_tokens"`
		CostUSD          float64 `json:"cost_usd"`
	}
	byModelRows := make([]modelRow, 0, len(byModel))
	for m, c := range byModel {
		byModelRows = append(byModelRows, modelRow{m, c.Requests, c.PromptTokens, c.CompletionTokens, c.CostUSD})
	}
	sort.Slice(byModelRows, func(i, j int) bool {
		if byModelRows[i].CostUSD != byModelRows[j].CostUSD {
			return byModelRows[i].CostUSD > byModelRows[j].CostUSD
		}
		return byModelRows[i].Model < byModelRows[j].Model
	})
	type dayRow struct {
		Date     string  `json:"date"`
		Requests int64   `json:"requests"`
		CostUSD  float64 `json:"cost_usd"`
	}
	dailyRows := make([]dayRow, 0, len(days))
	for _, d := range days {
		dailyRows = append(dailyRows, dayRow{d.Date, d.Requests, d.CostUSD})
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
		// Additive: what the vendors charged the gateway for the same calls, so
		// the operator's own dashboard can read both sides of the ledger. The
		// panel of a plain customer ignores it.
		"provider_cost_usd": providerCost,
		"by_model":          byModelRows,
		"daily":             dailyRows,
	})
}

// recentRequests answers the question the aggregate counters cannot: what did
// my key just do, and if it was refused, why.
//
// An administrator sees the whole deployment. Anyone else sees only calls
// attributed to a project one of their own keys owns — and the set of owned
// projects is built before the log is read, so a caller with no keys gets an
// empty set rather than a nil one, which is what the log treats as "everything".
func (s *Server) recentRequests(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value(consoleAdminCtxKey{}).(bool)
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)

	var mine map[string]bool
	if !isAdmin {
		mine = make(map[string]bool)
		for _, t := range s.admin.TokensForOwner(email) {
			mine[strings.ToLower(t.Name)] = true
		}
	}

	entries := s.requests.Recent(mine, 200)
	if entries == nil {
		entries = []adminstore.RequestEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requests": entries,
		// Said out loud because an empty log after a restart is not the same
		// fact as no traffic, and the console cannot tell the two apart.
		"volatile": true,
	})
}
