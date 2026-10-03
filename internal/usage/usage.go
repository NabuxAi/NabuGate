// Package usage tracks token consumption and cost per project and per model.
//
// Prices come from the config, in USD, keyed by "provider/model" for one
// provider's price, by the bare model name for the model's list price whoever
// serves it, or by "provider/*" as that provider's default (see Lookup).
package usage

import (
	"strings"
	"sync"

	"nabugate/internal/provider"
)

// Price is what a model's work costs, in USD, in the unit its vendor bills:
// tokens by direction, minutes of audio or of a live session, images made,
// characters of text turned into speech, or the vendor's own credits. A model is priced in the units its
// vendor actually charges in; the rest stay zero.
type Price struct {
	// Input and Output are per 1,000,000 tokens.
	Input  float64 `yaml:"input" json:"input,omitempty"`
	Output float64 `yaml:"output" json:"output,omitempty"`
	// PerMinute is per minute of audio transcribed or of a live session, billed
	// pro rata by the second.
	PerMinute float64 `yaml:"per_minute" json:"per_minute,omitempty"`
	// PerImage is per image generated.
	PerImage float64 `yaml:"per_image" json:"per_image,omitempty"`
	// PerMillionChars is per 1,000,000 characters of text spoken.
	PerMillionChars float64 `yaml:"per_million_chars" json:"per_million_chars,omitempty"`
	// PerCredit is per credit of a vendor that bills in its own credits.
	PerCredit float64 `yaml:"per_credit" json:"per_credit,omitempty"`
}

// Metered is one call's work in every unit a price can be quoted in. Most calls
// are tokens and nothing else.
type Metered struct {
	Tokens     provider.Usage
	Seconds    float64
	Images     int
	Characters int
}

// Tokens meters a call that is billed by its tokens alone.
func Tokens(u provider.Usage) Metered { return Metered{Tokens: u} }

// Identity is the name a model's list price is quoted under, whoever serves
// it: the last segment of the upstream name, lower-cased. Parspack's
// "openai/gpt-4o-mini", 9Router's "openrouter/openai/gpt-4o-mini" and AvalAI's
// "gpt-4o-mini" are one model at one list price.
func Identity(model string) string {
	if i := strings.LastIndex(model, "/"); i >= 0 {
		model = model[i+1:]
	}
	return strings.ToLower(strings.TrimSpace(model))
}

// Lookup finds the price of one upstream coordinate. Most specific first:
//
//  1. "provider/model" — what this provider charges for this model;
//  2. the model's identity — its list price, whoever serves it, so one entry
//     prices a model across the five resellers that serve it;
//  3. a ":free" model is free, which is what that suffix means on the routers
//     that use it;
//  4. "provider/*" — the provider's default, for the passthrough routers whose
//     catalogues of hundreds of models no config file can list.
//
// Not found means unpriced, and an unpriced call is metered at nothing. That
// is why config.UnpricedRoutes exists, and why the gateway warns at start-up.
func Lookup(prices map[string]Price, providerName, model string) (Price, bool) {
	if p, ok := prices[providerName+"/"+model]; ok {
		return p, true
	}
	id := Identity(model)
	if p, ok := prices[id]; ok {
		return p, true
	}
	if strings.HasSuffix(id, ":free") {
		return Price{}, true
	}
	p, ok := prices[providerName+"/*"]
	return p, ok
}

// Of is what a call's metered work costs at a price.
func (p Price) Of(m Metered) float64 {
	cost := float64(m.Tokens.PromptTokens)/1e6*p.Input + float64(m.Tokens.CompletionTokens)/1e6*p.Output
	if m.Seconds > 0 {
		cost += m.Seconds / 60 * p.PerMinute
	}
	if m.Images > 0 {
		cost += float64(m.Images) * p.PerImage
	}
	if m.Characters > 0 {
		cost += float64(m.Characters) / 1e6 * p.PerMillionChars
	}
	if m.Tokens.Credits > 0 {
		cost += float64(m.Tokens.Credits) * p.PerCredit
	}
	return cost
}

// Stat is the aggregated usage for a project or a model.
type Stat struct {
	Requests         int64   `json:"requests"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

func (s *Stat) add(requests int64, u provider.Usage, cost float64) {
	s.Requests += requests
	s.PromptTokens += int64(u.PromptTokens)
	s.CompletionTokens += int64(u.CompletionTokens)
	s.CostUSD += cost
}

// Tracker accumulates usage. It is safe for concurrent use.
type Tracker struct {
	mu        sync.Mutex
	prices    map[string]Price
	byProject map[string]*Stat
	byModel   map[string]*Stat
}

// New builds a Tracker with the given price table.
func New(prices map[string]Price) *Tracker {
	if prices == nil {
		prices = map[string]Price{}
	}
	return &Tracker{
		prices:    prices,
		byProject: map[string]*Stat{},
		byModel:   map[string]*Stat{},
	}
}

// Price is the price one upstream coordinate is billed at; see Lookup.
func (t *Tracker) Price(providerName, model string) (Price, bool) {
	return Lookup(t.prices, providerName, model)
}

// Cost returns the USD list cost of a call's metered work, 0 when the model is
// unpriced.
func (t *Tracker) Cost(providerName, model string, m Metered) float64 {
	p, ok := t.Price(providerName, model)
	if !ok {
		return 0
	}
	return p.Of(m)
}

// Record attributes a call's token usage and cost to the project and model,
// returning the computed cost (useful for logging).
func (t *Tracker) Record(project, providerName, model string, u provider.Usage) float64 {
	return t.RecordAt(project, providerName, model, 1, u, t.Cost(providerName, model, Tokens(u)))
}

// RecordAt meters a call at an explicit cost rather than the tracker's price
// for that model. It exists for work the gateway did not pay for: a request
// served by the caller's own vendor key still belongs in the usage numbers —
// the console should show what ran — but charging for it would bill the caller
// twice, once here and once by the vendor. requests is how many requests the
// call adds: none for a further usage report on a live session already counted.
func (t *Tracker) RecordAt(project, providerName, model string, requests int64, u provider.Usage, cost float64) float64 {
	if project == "" {
		project = "(unscoped)"
	}
	modelKey := providerName + "/" + model

	t.mu.Lock()
	defer t.mu.Unlock()
	t.statLocked(t.byProject, project).add(requests, u, cost)
	t.statLocked(t.byModel, modelKey).add(requests, u, cost)
	return cost
}

func (t *Tracker) statLocked(m map[string]*Stat, key string) *Stat {
	s := m[key]
	if s == nil {
		s = &Stat{}
		m[key] = s
	}
	return s
}

// Snapshot returns copies of the per-project and per-model aggregates.
func (t *Tracker) Snapshot() (byProject, byModel map[string]Stat) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return copyStats(t.byProject), copyStats(t.byModel)
}

// ProjectSnapshot returns the aggregate for a single project.
func (t *Tracker) ProjectSnapshot(project string) Stat {
	if project == "" {
		project = "(unscoped)"
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if s := t.byProject[project]; s != nil {
		return *s
	}
	return Stat{}
}

func copyStats(m map[string]*Stat) map[string]Stat {
	out := make(map[string]Stat, len(m))
	for k, v := range m {
		out[k] = *v
	}
	return out
}
