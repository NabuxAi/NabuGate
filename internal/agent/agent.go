// Package agent defines NabuGate sub-agents: named, reusable assistants that
// layer a system prompt and default sampling parameters on top of an existing
// chat model alias.
//
// Agents are declared entirely in configuration — inline under `agents:` in the
// main config, or as standalone YAML files in an `agents_dir` — so new
// specialists can be defined from outside the binary, with no code change, and
// invoked like any model through /v1/chat/completions (or /v1/responses). An
// agent does not talk to a provider itself: it resolves to an existing chat
// alias (or a "<provider>/<model>" passthrough) and rides the router's normal
// fallback chain, so a single request runs it end to end.
package agent

import (
	"fmt"
	"sort"
	"strings"
)

// Agent is one named sub-agent definition.
type Agent struct {
	// Name is the public identifier callers address as the request "model".
	Name string
	// Description is a short human-readable summary (surfaced on /v1/models).
	Description string
	// Model is the underlying chat alias (e.g. "nabu-smart") or a
	// "<provider>/<model>" passthrough the agent runs on.
	Model string
	// System is the system prompt injected ahead of the caller's messages.
	System string
	// Temperature/TopP/MaxTokens are defaults applied only when the caller did
	// not set them, so an explicit request value always wins.
	Temperature *float64
	TopP        *float64
	MaxTokens   *int
}

// Registry is a lookup of agents by name, populated once at startup.
type Registry struct {
	byName map[string]Agent
}

// NewRegistry returns an empty registry ready for Add.
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Agent)}
}

// Add registers an agent. It returns an error for an empty name, a missing
// model, or a duplicate name, so the caller can skip and warn instead of
// silently shadowing an earlier definition.
func (r *Registry) Add(a Agent) error {
	name := strings.TrimSpace(a.Name)
	if name == "" {
		return fmt.Errorf("agent has an empty name")
	}
	if strings.TrimSpace(a.Model) == "" {
		return fmt.Errorf("agent %q has no model", name)
	}
	if _, dup := r.byName[name]; dup {
		return fmt.Errorf("duplicate agent %q", name)
	}
	a.Name = name
	r.byName[name] = a
	return nil
}

// Lookup returns the agent registered under name. It is safe to call on a nil
// registry (returns not-found), so callers need not special-case the no-agents
// deployment.
func (r *Registry) Lookup(name string) (Agent, bool) {
	if r == nil {
		return Agent{}, false
	}
	a, ok := r.byName[name]
	return a, ok
}

// Names returns the registered agent names in sorted order.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.byName))
	for name := range r.byName {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Len reports how many agents are registered.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.byName)
}
