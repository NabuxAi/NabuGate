// Package usage tracks token consumption and cost per project and per model.
// Prices come from the config (USD per 1M tokens, keyed by "provider/model").
package usage

import (
	"sync"

	"nabugate/internal/provider"
)

// Price is the cost of a model in USD per 1,000,000 tokens.
type Price struct {
	Input  float64 `yaml:"input"`
	Output float64 `yaml:"output"`
}

// Stat is the aggregated usage for a project or a model.
type Stat struct {
	Requests         int64   `json:"requests"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

func (s *Stat) add(u provider.Usage, cost float64) {
	s.Requests++
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

// Cost returns the USD cost for a model's token usage (0 if the model is unpriced).
func (t *Tracker) Cost(providerName, model string, u provider.Usage) float64 {
	p, ok := t.prices[providerName+"/"+model]
	if !ok {
		return 0
	}
	return float64(u.PromptTokens)/1e6*p.Input + float64(u.CompletionTokens)/1e6*p.Output
}

// Record attributes a call's usage and cost to the project and model, returning
// the computed cost (useful for logging).
func (t *Tracker) Record(project, providerName, model string, u provider.Usage) float64 {
	cost := t.Cost(providerName, model, u)
	if project == "" {
		project = "(unscoped)"
	}
	modelKey := providerName + "/" + model

	t.mu.Lock()
	defer t.mu.Unlock()
	t.statLocked(t.byProject, project).add(u, cost)
	t.statLocked(t.byModel, modelKey).add(u, cost)
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
