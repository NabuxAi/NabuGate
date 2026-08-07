package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"nabugate/internal/policy"
)

type keyHealth struct {
	Status    string   `json:"status"`
	Project   string   `json:"project"`
	Degraded  int      `json:"degraded"`
	Allow     []string `json:"allow"`
	RateLimit int      `json:"rate_limit"`
	Aliases   []struct {
		ID        string   `json:"id"`
		Kind      string   `json:"kind"`
		Live      int      `json:"live_targets"`
		Providers []string `json:"providers"`
		Warnings  []string `json:"warnings"`
	} `json:"aliases"`
}

func getHealth(t *testing.T, url, key string) keyHealth {
	t.Helper()

	req, _ := http.NewRequest(http.MethodGet, url+"/v1/health", nil)
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/health = %d, want 200", resp.StatusCode)
	}

	var out keyHealth
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// A project reading its own health must learn what it may reach and what its
// allow-list is — the two facts that, until now, lived only in the console and
// cost a support conversation every time a caller named something outside it.
func TestKeyHealthReportsTheCallersOwnScope(t *testing.T) {
	up := fakeUpstream(t)
	defer up.Close()

	enforcer := policy.New(nil, []policy.KeyConfig{
		{Key: "desk", Project: "nabudesk", Allow: []string{"nabu-*"}, RateLimit: 120},
	})
	ts := newTestServer(t, up.URL, enforcer, nil)
	defer ts.Close()

	got := getHealth(t, ts.URL, "desk")

	if got.Project != "nabudesk" {
		t.Errorf("project = %q, want nabudesk", got.Project)
	}
	if len(got.Allow) != 1 || got.Allow[0] != "nabu-*" {
		t.Errorf("allow = %v, want [nabu-*]", got.Allow)
	}
	if got.RateLimit != 120 {
		t.Errorf("rate_limit = %d, want 120", got.RateLimit)
	}
	if len(got.Aliases) != 1 || got.Aliases[0].ID != "nabu-fast" {
		t.Fatalf("aliases = %+v, want just nabu-fast", got.Aliases)
	}
	if got.Aliases[0].Live != 1 {
		t.Errorf("live_targets = %d, want 1", got.Aliases[0].Live)
	}
}

// The endpoint is scoped, not a map of the whole gateway: an alias this key may
// not use is not this key's business, and the report names providers.
func TestKeyHealthHidesAliasesOutsideTheAllowList(t *testing.T) {
	up := fakeUpstream(t)
	defer up.Close()

	enforcer := policy.New(nil, []policy.KeyConfig{
		{Key: "elsewhere", Project: "other", Allow: []string{"write-*"}},
	})
	ts := newTestServer(t, up.URL, enforcer, nil)
	defer ts.Close()

	got := getHealth(t, ts.URL, "elsewhere")

	if len(got.Aliases) != 0 {
		t.Errorf("aliases = %+v, want none", got.Aliases)
	}
	// A key that can reach nothing is not "ok" — that is the state a project
	// spends a day debugging, and it should be one word.
	if got.Status != "unusable" {
		t.Errorf("status = %q, want unusable", got.Status)
	}
}

// A single-rung chat alias is a real risk and gets reported as degraded — one
// provider outage takes it down and nothing else on the gateway would say so.
func TestKeyHealthMarksAOneRungChainDegraded(t *testing.T) {
	up := fakeUpstream(t)
	defer up.Close()

	enforcer := policy.New(nil, []policy.KeyConfig{
		{Key: "desk", Project: "nabudesk", Allow: []string{"*"}},
	})
	ts := newTestServer(t, up.URL, enforcer, nil)
	defer ts.Close()

	got := getHealth(t, ts.URL, "desk")

	if got.Status != "degraded" || got.Degraded != 1 {
		t.Fatalf("status = %q degraded = %d, want degraded/1 for a fallback-less alias", got.Status, got.Degraded)
	}
	if len(got.Aliases[0].Warnings) == 0 {
		t.Error("a degraded alias must carry the reason, not just the count")
	}
}

func TestKeyHealthNeedsAKey(t *testing.T) {
	up := fakeUpstream(t)
	defer up.Close()

	enforcer := policy.New(nil, []policy.KeyConfig{{Key: "desk", Project: "nabudesk"}})
	ts := newTestServer(t, up.URL, enforcer, nil)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// It reports which providers are keyed and which are not. That is a map of
	// the gateway's own weak points, and it goes behind the same auth as
	// everything else.
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated GET /v1/health = %d, want 401", resp.StatusCode)
	}
}
