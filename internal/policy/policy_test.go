package policy

import "testing"

// TestAdminOnlyForSimpleKeys guards the /v1/usage visibility rule: only the
// simple full-access api_keys are admins. A rich key that omits `project:` must
// NOT be treated as admin (that would leak every project's usage).
func TestAdminOnlyForSimpleKeys(t *testing.T) {
	e := New([]string{"admin_key"}, []KeyConfig{
		{Key: "scoped_key", Project: "crm", Allow: []string{"nabu-fast"}},
		{Key: "no_project_key", Allow: []string{"*"}}, // rich key, project omitted
	})

	cases := map[string]bool{
		"admin_key":      true,
		"scoped_key":     false,
		"no_project_key": false,
	}
	for key, wantAdmin := range cases {
		pol, ok := e.Lookup(key)
		if !ok {
			t.Fatalf("key %q not found", key)
		}
		if pol.Admin != wantAdmin {
			t.Errorf("key %q: Admin = %v, want %v", key, pol.Admin, wantAdmin)
		}
	}
}

// TestEmptyKeysDisableAuth documents that blank keys are skipped, so an unset
// ${VAR} in api_keys can't become a usable empty-string token.
func TestEmptyKeysDisableAuth(t *testing.T) {
	e := New([]string{""}, []KeyConfig{{Key: ""}})
	if e.Enabled() {
		t.Error("Enabled() = true for all-blank keys, want false (no valid tokens)")
	}
}

// TestAllowsGlobsAndProviderWildcards covers both the existing flat-alias glob
// behavior and the provider-namespace grants used for passthrough models, where
// a model ID like "openai/gpt-5.5" is itself nested under the provider.
func TestAllowsGlobsAndProviderWildcards(t *testing.T) {
	cases := []struct {
		name    string
		allow   []string
		alias   string
		allowed bool
	}{
		{"empty allows all", nil, "parspack/openai/gpt-5.5", true},
		{"star allows all", []string{"*"}, "parspack/openai/gpt-5.5", true},
		{"flat glob matches", []string{"nabu-*"}, "nabu-fast", true},
		{"flat exact mismatch", []string{"nabu-fast"}, "nabu-smart", false},
		{"provider /* matches nested id", []string{"parspack/*"}, "parspack/openai/gpt-5.5", true},
		{"provider /* matches single segment", []string{"parspack/*"}, "parspack/grok", true},
		{"provider /** matches nested id", []string{"parspack/**"}, "parspack/openai/gpt-5.5", true},
		{"provider /* rejects other provider", []string{"parspack/*"}, "dahl/openai/gpt", false},
		{"provider /* rejects bare provider suffix", []string{"parspack/*"}, "parspackery/x", false},
		{"mixed list", []string{"nabu-fast", "parspack/*"}, "parspack/anthropic/claude-sonnet-4.6", true},
	}
	for _, c := range cases {
		p := Policy{Allow: c.allow}
		if got := p.Allows(c.alias); got != c.allowed {
			t.Errorf("%s: Allow=%v alias=%q -> %v, want %v", c.name, c.allow, c.alias, got, c.allowed)
		}
	}
}
