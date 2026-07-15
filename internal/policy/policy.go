// Package policy implements per-API-key access control for NabuGate: which
// aliases a key may use (allow-list with glob support) and an optional
// per-key request rate limit (token bucket, requests per minute).
package policy

import (
	"path"
	"strings"
	"sync"
	"time"
)

// KeyConfig is the rich form of an API key in the config file.
type KeyConfig struct {
	Key       string   `yaml:"key"`
	Project   string   `yaml:"project"`
	Allow     []string `yaml:"allow"`      // alias globs; "*" (or empty) allows all
	RateLimit int      `yaml:"rate_limit"` // requests per minute; 0 = unlimited
}

// Policy is the resolved access policy attached to a key.
type Policy struct {
	Admin     bool // full-access simple api_key: may see every project's usage
	Project   string
	Allow     []string
	RateLimit int
}

// Allows reports whether the policy permits the given alias. An empty allow-list
// or a "*" entry permits everything; otherwise each entry is matched as a glob.
func (p Policy) Allows(alias string) bool {
	if len(p.Allow) == 0 {
		return true
	}
	for _, pattern := range p.Allow {
		if matchAlias(pattern, alias) {
			return true
		}
	}
	return false
}

// matchAlias matches one allow-list pattern against an alias. "*" (or "**")
// permits everything. A pattern ending in "/*" or "/**" grants a whole
// passthrough-provider namespace: "parspack/*" matches "parspack/openai/gpt-5.5"
// too, which path.Match alone would miss because its "*" does not cross the "/"
// separator (provider model IDs like "openai/gpt-5.5" are themselves nested).
// Every other pattern keeps path.Match glob semantics, so flat aliases such as
// "nabu-*" behave exactly as before.
func matchAlias(pattern, alias string) bool {
	if pattern == "*" || pattern == "**" {
		return true
	}
	if prefix, ok := strings.CutSuffix(pattern, "/**"); ok {
		return alias == prefix || strings.HasPrefix(alias, prefix+"/")
	}
	if prefix, ok := strings.CutSuffix(pattern, "/*"); ok {
		return alias == prefix || strings.HasPrefix(alias, prefix+"/")
	}
	if ok, err := path.Match(pattern, alias); err == nil && ok {
		return true
	}
	return false
}

// Enforcer resolves keys to policies and enforces per-key rate limits.
type Enforcer struct {
	policies map[string]Policy

	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

// New builds an Enforcer. simpleKeys (the legacy `api_keys` list) get full
// access with no rate limit; richKeys (the `keys` list) carry explicit policies
// and override a simple key with the same value.
func New(simpleKeys []string, richKeys []KeyConfig) *Enforcer {
	policies := make(map[string]Policy)
	for _, k := range simpleKeys {
		if k != "" {
			policies[k] = Policy{Admin: true, Allow: []string{"*"}}
		}
	}
	for _, k := range richKeys {
		if k.Key == "" {
			continue
		}
		policies[k.Key] = Policy{Project: k.Project, Allow: k.Allow, RateLimit: k.RateLimit}
	}
	return &Enforcer{
		policies: policies,
		buckets:  make(map[string]*bucket),
		now:      time.Now,
	}
}

// Enabled reports whether any keys are configured. When false, the gateway runs
// in open (dev) mode with no auth.
func (e *Enforcer) Enabled() bool { return len(e.policies) > 0 }

// Lookup returns the policy for a key.
func (e *Enforcer) Lookup(key string) (Policy, bool) {
	p, ok := e.policies[key]
	return p, ok
}

// RateOK consumes one token for the key, returning false if the per-minute rate
// limit is exceeded. Keys with RateLimit <= 0 are always allowed.
func (e *Enforcer) RateOK(key string) bool {
	pol, ok := e.policies[key]
	if !ok || pol.RateLimit <= 0 {
		return true
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.buckets[key]
	if b == nil {
		capacity := float64(pol.RateLimit)
		b = &bucket{tokens: capacity, capacity: capacity, refill: capacity / 60.0, last: e.now()}
		e.buckets[key] = b
	}
	return b.allow(e.now())
}

// bucket is a simple token bucket refilling at refill tokens/second.
type bucket struct {
	tokens   float64
	capacity float64
	refill   float64
	last     time.Time
}

func (b *bucket) allow(now time.Time) bool {
	elapsed := now.Sub(b.last).Seconds()
	b.last = now
	b.tokens += elapsed * b.refill
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}
