package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"nabugate/internal/adminstore"
	"nabugate/internal/config"
)

// --- Subscriptions, over HTTP ---
//
// Buying a plan is a purchase against the balance the user already topped up,
// not a second payment rail: the console's top-up screen puts money in, and
// this spends some of it on a month of access to the gateway's own vendor keys.
// One balance, two things it pays for — access, and then the metered calls that
// access allows.

// planView is one offer as the console shows it. The config type is not sent
// straight out: `providers` are the deployment's routing names, and publishing
// the glob list would tell every signed-in user which upstreams exist by what
// this gateway calls them, which is the catalogue's job and not a price list's.
type planView struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	PriceUSD    float64 `json:"price_usd"`
	Days        int     `json:"days"`
	Markup      float64 `json:"markup"`
	MinCharge   float64 `json:"min_charge_usd,omitempty"`
	Credit      float64 `json:"includes_credit_usd,omitempty"`
	Popular     bool    `json:"popular,omitempty"`
	// Providers is how many upstreams the plan unlocks here, and whether that
	// is all of them — enough to compare two plans without naming internals.
	Unlocks    int  `json:"unlocks"`
	UnlocksAll bool `json:"unlocks_all"`
}

// SetPlans hands the server the subscriptions this deployment sells.
func (s *Server) SetPlans(p []config.Plan) { s.plans = p }

func (s *Server) planViews() []planView {
	out := make([]planView, 0, len(s.plans))
	for _, p := range s.plans {
		v := planView{
			ID: p.ID, Name: p.Name, Description: p.Description,
			PriceUSD: p.PriceUSD, Days: p.Term(), Markup: p.Rate(),
			MinCharge: p.MinChargeUSD, Credit: p.IncludesCreditUSD,
			Popular: p.Popular,
		}
		probe := adminstore.Subscription{Providers: p.Providers}
		for name := range s.providers {
			if probe.Covers(name) {
				v.Unlocks++
			}
		}
		for _, g := range p.Providers {
			if strings.TrimSpace(g) == "*" {
				v.UnlocksAll = true
			}
		}
		out = append(out, v)
	}
	return out
}

// listPlans is the buy screen: what is on offer, and what you already hold.
func (s *Server) listPlans(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	body := map[string]any{"plans": s.planViews()}
	if s.admin != nil && email != "" {
		if sub, ok := s.admin.SubscriptionOf(email); ok {
			body["subscription"] = sub
			// Held and in force are different facts, and a console that showed
			// only the first would tell a user with a lapsed plan that they
			// have one.
			body["active"] = sub.Active(time.Now().UTC())
		}
		if u := s.admin.GetUser(email); u != nil {
			body["balance"] = u.Balance
		}
	}
	writeJSON(w, http.StatusOK, body)
}

// subscribe buys one term against the user's balance.
func (s *Server) subscribe(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	id := strings.ToLower(strings.TrimSpace(r.PathValue("id")))

	var plan config.Plan
	found := false
	for _, p := range s.plans {
		if strings.EqualFold(strings.TrimSpace(p.ID), id) {
			plan, found = p, true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "no such plan")
		return
	}
	if len(plan.Providers) == 0 {
		// A plan that unlocks nothing is a configuration mistake, and selling
		// it would take the money and change nothing the buyer could see.
		writeError(w, http.StatusServiceUnavailable, "that plan is not available right now")
		return
	}

	sub, err := s.admin.Subscribe(email, adminstore.Subscription{
		PlanID:       plan.ID,
		Name:         plan.Name,
		PriceUSD:     plan.PriceUSD,
		Markup:       plan.Rate(),
		MinChargeUSD: plan.MinChargeUSD,
		Providers:    plan.Providers,
	}, plan.Term())
	if err != nil {
		if err == adminstore.ErrInsufficientBalance {
			writeError(w, http.StatusPaymentRequired, "top up your balance first — this plan costs more than you have")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if plan.IncludesCreditUSD > 0 {
		_ = s.admin.AddPayment(email, plan.IncludesCreditUSD, "plan-credit", "plan:"+plan.ID)
	}
	s.log.Info("subscribed", "owner", email, "plan", plan.ID, "price", plan.PriceUSD, "until", sub.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{"subscription": sub})
}

// cancelSubscription ends it now. No refund is computed here: a part-used term
// is a decision for a person, and quietly returning some fraction would be a
// policy invented by a handler.
func (s *Server) cancelSubscription(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	if err := s.admin.Unsubscribe(email); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.log.Info("subscription cancelled", "owner", email)
	writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
}

// adminSetSubscription grants or ends a subscription without payment, for the
// support case the buy flow cannot cover: a customer who paid another way, or
// one owed a term after an outage.
func (s *Server) adminSetSubscription(w http.ResponseWriter, r *http.Request) {
	by, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	var body struct {
		Email  string `json:"email"`
		PlanID string `json:"plan_id"`
		Days   int    `json:"days"`
		Cancel bool   `json:"cancel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Email) == "" {
		writeError(w, http.StatusBadRequest, "send {\"email\":..., \"plan_id\":...}")
		return
	}
	if body.Cancel {
		if err := s.admin.Unsubscribe(body.Email); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.log.Info("subscription cancelled by admin", "owner", body.Email, "by", by)
		writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
		return
	}
	var plan config.Plan
	found := false
	for _, p := range s.plans {
		if strings.EqualFold(strings.TrimSpace(p.ID), strings.TrimSpace(body.PlanID)) {
			plan, found = p, true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "no such plan")
		return
	}
	days := body.Days
	if days <= 0 {
		days = plan.Term()
	}
	// PriceUSD zero: the balance is not touched, because this is the grant path
	// and the money — if there was any — arrived somewhere else.
	sub, err := s.admin.Subscribe(body.Email, adminstore.Subscription{
		PlanID:       plan.ID,
		Name:         plan.Name,
		Markup:       plan.Rate(),
		MinChargeUSD: plan.MinChargeUSD,
		Providers:    plan.Providers,
	}, days)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.log.Info("subscription granted", "owner", body.Email, "plan", plan.ID, "days", days, "by", by)
	writeJSON(w, http.StatusOK, map[string]any{"subscription": sub})
}
