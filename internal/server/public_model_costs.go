package server

import (
	"net/http"
	"sort"

	"nabugate/internal/usage"
	"nabugate/internal/adminstore"
)

// A quote catalog, not a price promise. The gateway may fall back to another
// upstream with a different rate, and provider-specific tier rules may differ
// from the deployment's configured list. The response never exposes provider
// credentials or internal routing names.
//
// This list is public so visitors can estimate costs *before* creating an
// account. Configured subscription fees and token rates are intentionally
// shown separately: a $9 access term and a 40% per-call markup are two charges,
// not one "unlimited token pack".
type publicModelCosts struct {
	Alias        string        `json:"alias"`
	Rates        []usage.Price `json:"rates"`
	TotalRoutes  int           `json:"total_routes"`
	PricedRoutes int           `json:"priced_routes"`
	Complete     bool          `json:"complete"`
}

type publicCostPlan struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	PriceUSD       float64 `json:"price_usd"`
	Days           int     `json:"days"`
	Markup         float64 `json:"markup"`
	MinChargeUSD   float64 `json:"min_charge_usd"`
	IncludesCredit float64 `json:"includes_credit_usd"`
}

func (s *Server) publicModelCosts(w http.ResponseWriter, r *http.Request) {
	if s.router == nil || s.usage == nil {
		writeError(w, http.StatusServiceUnavailable, "pricing catalog not configured")
		return
	}
	models := make([]publicModelCosts, 0)
	for _, info := range s.router.Routes() {
		if info.Kind != "chat" {
			continue
		}
		view := s.aliasView(info, adminstore.AliasSetting{})
		row := publicModelCosts{Alias: view.Alias, Rates: []usage.Price{}}
		for _, rung := range view.Rungs {
			for _, c := range rung.Coordinates {
				row.TotalRoutes++
				if c.Priced {
					row.PricedRoutes++
					row.Rates = append(row.Rates, c.Price)
				}
			}
		}
		// A zero-priced route is different from an unknown rate: a legitimate
		// explicit {input:0,output:0} is included and complete.
		row.Complete = row.TotalRoutes > 0 && row.TotalRoutes == row.PricedRoutes
		models = append(models, row)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Alias < models[j].Alias })
	plans := make([]publicCostPlan, 0, len(s.plans))
	for _, p := range s.plans {
		plans = append(plans, publicCostPlan{
			ID: p.ID, Name: p.Name, PriceUSD: p.PriceUSD, Days: p.Term(),
			Markup: p.Rate(), MinChargeUSD: p.MinChargeUSD,
			IncludesCredit: p.IncludesCreditUSD,
		})
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, map[string]any{
		"currency": "USD",
		"source": "deployed_config",
		"pricing_is_quote": false,
		"rates_are_per_million_tokens": true,
		"proxy_fee_usd": 0,
		"proxy_fee_note": "The current gateway's processing overhead is included in the subscription markup/minimum; no additional proxy surcharge is configured.",
		"byok_note": "BYOK provider spend is billed directly by the upstream and is not deducted from the NabuGate wallet.",
		"warning": "Estimates exclude unconfigured tiers, exact provider caching, retries, exchange-rate settlement and source-side price changes. Routing may select any priced or unpriced fallback.",
		"models": models, "plans": plans,
	})
}
