// Package flow defines NabuGate flows: named chains of agents where each step
// receives what the one before it produced.
//
// A sub-agent is one specialist answering once. A flow is several of them in a
// stated order — read the history, draft an approach, argue with the draft,
// then sum it up — and the order is the whole point. The difference from one
// long prompt asking a single model to also check its own work is that the
// reviewer is a separate call which never saw itself write the draft, so it can
// actually disagree.
//
// Like agents, flows are declared entirely outside the binary — inline under
// `flows:` in the main config, as YAML in a `flows_dir`, or created from the
// console — and are addressable as a request "model", so any OpenAI-compatible
// client runs a whole chain in one call and needs to know nothing about it.
package flow

import (
	"fmt"
	"sort"
	"strings"
)

// Placeholders a step's Input template may use.
const (
	// PlaceholderPrevious is what the step before this one produced. On the
	// first step it is empty.
	PlaceholderPrevious = "{{previous}}"
	// PlaceholderInput is the caller's own last user message, so a late step
	// can look back at what was originally asked rather than only at what the
	// step before it made of it.
	PlaceholderInput = "{{input}}"
)

// Step is one link in the chain.
type Step struct {
	// Agent is the name of a sub-agent, a chat alias, or another flow.
	Agent string
	// Label names the step in the trace. Defaults to Agent.
	Label string
	// Input is what this step is handed, as a template. Empty means the
	// previous step's output verbatim — which is the common case and the one
	// worth not making people write out.
	Input string
	// Optional marks a step whose failure does not end the chain: its output is
	// skipped and the next step receives whatever the last successful one
	// produced. Off by default, because a reviewer handed nothing writes a
	// review of nothing and it reads exactly as confidently as a real one.
	Optional bool
}

// Flow is one named chain.
type Flow struct {
	Name        string
	Description string
	Steps       []Step
}

// Render fills a step's template. An empty template means "just the previous
// output", so the ordinary chain needs no template at all.
func (s Step) Render(previous, input string) string {
	tpl := strings.TrimSpace(s.Input)
	if tpl == "" {
		return previous
	}

	tpl = strings.ReplaceAll(tpl, PlaceholderPrevious, previous)
	tpl = strings.ReplaceAll(tpl, PlaceholderInput, input)

	return tpl
}

// Name of the step as it appears in a trace.
func (s Step) DisplayName() string {
	if label := strings.TrimSpace(s.Label); label != "" {
		return label
	}

	return s.Agent
}

// Registry is a lookup of flows by name, populated at startup and mutated by
// the console. It mirrors agent.Registry deliberately: two lookups with
// different rules would be two places to get the precedence wrong.
type Registry struct {
	byName map[string]Flow
}

func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Flow)}
}

// Add registers a flow, refusing a duplicate so a second definition cannot
// silently shadow the first.
func (r *Registry) Add(f Flow) error {
	name, err := validate(f)
	if err != nil {
		return err
	}
	if _, dup := r.byName[name]; dup {
		return fmt.Errorf("duplicate flow %q", name)
	}

	f.Name = name
	r.byName[name] = f

	return nil
}

// Set is Add that overwrites, for flows edited at runtime from the console.
func (r *Registry) Set(f Flow) error {
	name, err := validate(f)
	if err != nil {
		return err
	}

	f.Name = name
	r.byName[name] = f

	return nil
}

func validate(f Flow) (string, error) {
	name := strings.TrimSpace(f.Name)
	if name == "" {
		return "", fmt.Errorf("flow has an empty name")
	}
	if len(f.Steps) == 0 {
		return "", fmt.Errorf("flow %q has no steps", name)
	}
	for i, step := range f.Steps {
		if strings.TrimSpace(step.Agent) == "" {
			return "", fmt.Errorf("flow %q step %d names no agent", name, i+1)
		}
	}

	return name, nil
}

func (r *Registry) Remove(name string) {
	if r == nil {
		return
	}
	delete(r.byName, strings.TrimSpace(name))
}

// Lookup is safe on a nil registry, so the no-flows deployment needs no
// special case at any call site.
func (r *Registry) Lookup(name string) (Flow, bool) {
	if r == nil {
		return Flow{}, false
	}
	f, ok := r.byName[strings.TrimSpace(name)]

	return f, ok
}

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

func (r *Registry) Len() int {
	if r == nil {
		return 0
	}

	return len(r.byName)
}
