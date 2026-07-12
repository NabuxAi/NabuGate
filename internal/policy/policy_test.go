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
