package config

import (
	"sort"
	"strings"
)

// LiveProblems names every live alias that must not be served, and why.
//
// A live minute is billed from the pricing table and from nowhere else, so a
// rung with no per_minute price is not an error anywhere on the request path:
// the call connects, runs, and every second of it bills the caller nothing
// while the vendor bills the gateway. That is checked once, at start-up, and
// the alias is refused rather than the gateway — one misconfigured voice alias
// must not take chat down with it.
func (c *Config) LiveProblems() map[string]string {
	problems := make(map[string]string)
	for alias, route := range c.Live {
		if route.Primary.Provider == "" && route.Primary.Model == "" {
			problems[alias] = "no primary target"
			continue
		}
		var unpriced, unserved []string
		for _, t := range append([]Target{route.Primary}, route.Fallback...) {
			coords, ok := c.liveCoordinates(t)
			if !ok {
				unserved = append(unserved, t.Model)
				continue
			}
			for _, coord := range coords {
				if p, priced := c.Pricing[coord]; !priced || p.PerMinute <= 0 {
					unpriced = append(unpriced, coord)
				}
			}
		}
		var reasons []string
		if len(unpriced) > 0 {
			sort.Strings(unpriced)
			reasons = append(reasons, "no per_minute price for "+strings.Join(unpriced, ", "))
		}
		if len(unserved) > 0 {
			sort.Strings(unserved)
			reasons = append(reasons, "no provider serves "+strings.Join(unserved, ", "))
		}
		if len(reasons) > 0 {
			problems[alias] = strings.Join(reasons, "; ")
		}
	}
	return problems
}

// liveCoordinates is every "provider/model" pricing key one live rung can be
// billed under: its own, or — for a rung naming only a model — one per
// provider the model registry says serves it. ok is false for a model-only
// rung the registry does not know.
func (c *Config) liveCoordinates(t Target) ([]string, bool) {
	if t.Provider != "" {
		return []string{t.Provider + "/" + t.Model}, true
	}
	entry, ok := c.Registry[t.Model]
	if !ok || len(entry.Serves) == 0 {
		return nil, false
	}
	out := make([]string, 0, len(entry.Serves))
	for _, s := range entry.Serves {
		out = append(out, s.Provider+"/"+s.Model)
	}
	return out, true
}
