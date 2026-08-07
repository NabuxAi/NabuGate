package router

import (
	"sort"

	"nabugate/internal/config"
)

// AliasHealth is one alias as it stands right now, rather than as configured.
//
// /v1/models answers "may this key use that name". It cannot answer the
// question that actually breaks callers: whether anything is left behind the
// name. An alias whose only provider lost its key is still permitted, still
// listed, and fails every request with "all targets failed" — which the caller
// discovers by spending a request and reading an error written for us, not for
// them.
type AliasHealth struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // chat | image | audio | embedding

	// Live is how many concrete upstream attempts this alias resolves to today,
	// after registry expansion and after dropping providers with no key.
	Live int `json:"live_targets"`

	// Configured is how many rungs the config declares. Live < Configured means
	// the chain is shorter than its author intended.
	Configured int `json:"configured_targets"`

	// Providers is the distinct vendors still standing behind this alias, in
	// the order they would be tried.
	Providers []string `json:"providers"`

	// Warnings are the ways this alias is weaker than it looks. Empty is the
	// healthy case.
	Warnings []string `json:"warnings,omitempty"`
}

// Healthy reports whether anything at all can serve this alias.
func (a AliasHealth) Healthy() bool { return a.Live > 0 }

// AliasHealthAll reports every configured alias's standing.
//
// Deliberately config-only: no upstream is contacted. A health endpoint that
// calls every provider costs money on every poll and is the first thing to be
// switched off, which leaves nothing. What this catches instead is the whole
// class of faults that are already decided before a request is made — a
// provider whose key never made it into this deployment's env, a fallback that
// never crossed vendors, a chain that quietly wore down to one rung.
func (r *Router) AliasHealthAll() []AliasHealth {
	var out []AliasHealth

	kinds := []struct {
		name     string
		registry map[string]config.ModelRoute
	}{
		{"chat", r.models},
		{"image", r.images},
		{"audio", r.audio},
		{"embedding", r.embeddings},
		{"transcription", r.transcription},
	}

	for _, kind := range kinds {
		for alias, route := range kind.registry {
			out = append(out, r.aliasHealth(alias, kind.name, route))
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})

	return out
}

func (r *Router) aliasHealth(alias, kind string, route config.ModelRoute) AliasHealth {
	rungs := append([]config.Target{route.Primary}, route.Fallback...)

	health := AliasHealth{ID: alias, Kind: kind, Configured: len(rungs)}

	seen := make(map[string]bool, len(rungs))

	for _, rung := range rungs {
		// expand() is the same call the real request path makes, so a registry
		// rung fans out into one attempt per serving provider exactly as a real
		// request would.
		for _, target := range r.expand(rung) {
			// A rung naming a provider directly is *not* filtered by expand —
			// the live request path checks the adapter at attempt time and
			// records "provider not available". Counting it here would report a
			// fallback that only ever produces that error as a working rung,
			// which is the exact fiction this endpoint exists to end.
			if _, live := r.adapters[target.Provider]; !live {
				continue
			}

			health.Live++

			if !seen[target.Provider] {
				seen[target.Provider] = true
				health.Providers = append(health.Providers, target.Provider)
			}
		}
	}

	switch {
	case health.Live == 0:
		health.Warnings = append(health.Warnings, "nothing can serve this alias — every provider behind it is missing its API key in this deployment")
	case len(health.Providers) == 1 && health.Live > 1:
		// Two models behind one key share one quota and one outage: that is one
		// target, not two, and a chain of them fails twice at once.
		health.Warnings = append(health.Warnings, "fallback does not cross vendors — every rung is served by "+health.Providers[0])
	case health.Live == 1 && kind != "embedding":
		// An embedding alias with one rung is usually deliberate: crossing
		// vector widths mid-flight corrupts a stored index, so no fallback is
		// the correct configuration there and warning about it is noise.
		health.Warnings = append(health.Warnings, "no fallback — one provider outage takes this alias down")
	}

	if health.Live < health.Configured && health.Live > 0 {
		health.Warnings = append(health.Warnings, "shorter than configured — some rungs name providers with no key here")
	}

	return health
}
