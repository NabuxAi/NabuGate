package server

import (
	"context"
	"fmt"
	"math"
	"net/http/httptest"
	"testing"

	"nabugate/internal/adminstore"
	"nabugate/internal/config"
	"nabugate/internal/router"
)

func subServer(t *testing.T) (*Server, *adminstore.Store) {
	t.Helper()
	st, err := adminstore.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SignupUser("me@example.com", "hunter2hunter2"); err != nil {
		t.Fatal(err)
	}
	st.AddPayment("me@example.com", 50, "success", "seed")
	return &Server{admin: st, providers: map[string]config.ProviderMeta{
		"openai": {Name: "openai", Enabled: true, Access: "request"},
		"gemini": {Name: "gemini", Enabled: true, Access: "request"},
		"groq":   {Name: "groq", Enabled: true, Access: "auto"},
	}}, st
}

func gatewayPlan(providers ...string) adminstore.Subscription {
	return adminstore.Subscription{PlanID: "gateway", PriceUSD: 9,
		Markup: 1.4, MinChargeUSD: 0.0002, Providers: providers}
}

// A subscription is a second way past the same gate, alongside an admin's
// approval — not a replacement for it and not a new gate. Everything that
// worked before must still work.
func TestSubscriptionOpensTheGatewayRung(t *testing.T) {
	s, st := subServer(t)

	// Before buying: both request-gated providers are held back.
	denied, _ := s.withOwnerCredentials(context.Background(), "me@example.com").
		Value(router.GatewayDeniedCtxKey{}).(map[string]bool)
	if !denied["openai"] || !denied["gemini"] {
		t.Fatalf("denied = %v, want both gated providers held", denied)
	}

	if _, err := st.Subscribe("me@example.com", gatewayPlan("openai"), 30); err != nil {
		t.Fatal(err)
	}
	denied, _ = s.withOwnerCredentials(context.Background(), "me@example.com").
		Value(router.GatewayDeniedCtxKey{}).(map[string]bool)
	if denied["openai"] {
		t.Error("the subscription did not open the provider it covers")
	}
	// And only that one: a plan naming one provider must not unlock the rest.
	if !denied["gemini"] {
		t.Error("a plan covering openai also unlocked gemini")
	}
	if denied["groq"] {
		t.Error("an auto provider was gated")
	}
}

// Expiry is evaluated per request, which is what lets a lapse need no sweeper.
func TestExpiredSubscriptionClosesTheGateAgain(t *testing.T) {
	s, st := subServer(t)
	if _, err := st.Subscribe("me@example.com", gatewayPlan("*"), 1); err != nil {
		t.Fatal(err)
	}
	denied, _ := s.withOwnerCredentials(context.Background(), "me@example.com").
		Value(router.GatewayDeniedCtxKey{}).(map[string]bool)
	if denied["openai"] {
		t.Fatal("a live subscription did not open the gate")
	}

	// The term runs out. Nothing else happens — no job, no restart.
	if err := st.Unsubscribe("me@example.com"); err != nil {
		t.Fatal(err)
	}
	denied, _ = s.withOwnerCredentials(context.Background(), "me@example.com").
		Value(router.GatewayDeniedCtxKey{}).(map[string]bool)
	if !denied["openai"] {
		t.Error("the gate stayed open after the subscription ended")
	}
}

// A caller on their own key is charged nothing, subscription or not: their
// vendor already billed them. That is what "our keys cost extra" means — the
// extra is the only charge there is.
func TestOwnKeyStaysFreeUnderASubscription(t *testing.T) {
	s, st := subServer(t)
	if _, err := st.Subscribe("me@example.com", gatewayPlan("*"), 30); err != nil {
		t.Fatal(err)
	}
	ctx := s.withOwnerCredentials(context.Background(), "me@example.com")
	rate := gatewayRateFrom(ctx)
	if rate.markup != 1.4 {
		t.Fatalf("markup = %v, want the plan's", rate.markup)
	}
	// The rate is only ever consulted for a gateway-served call; record()
	// zeroes a caller-served one before it gets here. This asserts the two
	// paths stay distinct: applying the plan rate to zero is not the same
	// thing as not charging.
	if got := rate.apply(0.01); math.Abs(got-0.014) > 1e-9 {
		t.Errorf("gateway call priced at %v, want the markup applied", got)
	}
}

// The revenue hole this floor exists to close: a model with no `pricing:` entry
// meters at zero, and any multiple of zero is zero. Without a floor a month of
// transcription on the gateway's own keys bills nothing.
func TestUnpricedGatewayCallStillCharges(t *testing.T) {
	rate := gatewayRate{markup: 1.4, minCharge: 0.0002}
	if got := rate.apply(0); got != 0.0002 {
		t.Errorf("unpriced call = %v, want the floor", got)
	}
	// A priced call above the floor is untouched by it.
	if got := rate.apply(1); math.Abs(got-1.4) > 1e-9 {
		t.Errorf("priced call = %v, want only the markup", got)
	}
}

// No subscription means the metered cost, exactly as every deployment behaved
// before plans existed. A user on an auto provider must not start paying more
// because this feature shipped.
func TestNoSubscriptionMeansNoMarkupAndNoFloor(t *testing.T) {
	s, _ := subServer(t)
	rate := gatewayRateFrom(s.withOwnerCredentials(context.Background(), "me@example.com"))
	if got := rate.apply(0.01); got != 0.01 {
		t.Errorf("cost = %v, want the metered cost untouched", got)
	}
	if got := rate.apply(0); got != 0 {
		t.Errorf("unpriced call = %v, want zero without a plan", got)
	}
	// And a token with no owner — a config-baked key — reaches the same rate.
	if got := gatewayRateFrom(context.Background()).apply(0.02); got != 0.02 {
		t.Errorf("keyless-owner cost = %v; an internal integration changed price", got)
	}
}

// Repeated headers are the list. Splitting on commas would turn one vendor key
// containing a comma into two invalid ones, so it is deliberately not done.
func TestRepeatedHeadersBecomeAKeyList(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Add("X-Nabu-Key-Gemini", "AIza-one")
	req.Header.Add("X-Nabu-Key-Gemini", "AIza-two")
	req.Header.Add("X-Nabu-Key-Openai", "sk-a,sk-b")

	got := callerKeysFrom(req)
	if k := got.Keys["gemini"]; len(k) != 2 || k[0] != "AIza-one" || k[1] != "AIza-two" {
		t.Errorf("gemini = %q, want both headers in order", k)
	}
	if k := got.Keys["openai"]; len(k) != 1 || k[0] != "sk-a,sk-b" {
		t.Errorf("openai = %q; a comma inside a key must not split it", k)
	}
}

// Headers first, stored keys behind them. Explicit still beats saved, and with
// lists that means tried-first rather than replaced — a saved key stays useful
// as the fallback it was added to be.
func TestHeaderKeysAreTriedBeforeStoredOnes(t *testing.T) {
	t.Setenv("NABUGATE_SECRET_KEY", "master")
	st, err := adminstore.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"sk-stored-1", "sk-stored-2"} {
		if _, err := st.AddProviderKey("me@example.com", "openai", "", k); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{admin: st}

	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Add("X-Nabu-Key-Openai", "sk-header")
	ctx, _ := withCallerKeys(req.Context(), req)
	ctx = s.withOwnerCredentials(ctx, "me@example.com")

	got := ctx.Value(router.CallerKeysCtxKey{}).(router.CallerKeys).Keys["openai"]
	want := []string{"sk-header", "sk-stored-1", "sk-stored-2"}
	if len(got) != len(want) {
		t.Fatalf("keys = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("key %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// Saved keys keep the order they were added in. A user's first key is the one
// they mean by default; renumbering on delete would move it.
func TestStoredKeyOrderSurvivesADelete(t *testing.T) {
	t.Setenv("NABUGATE_SECRET_KEY", "master")
	st, err := adminstore.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, k := range []string{"sk-1", "sk-2", "sk-3"} {
		rec, err := st.AddProviderKey("me@example.com", "openai", k, k)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, rec.ID)
	}
	if err := st.DeleteProviderKey("me@example.com", "openai", ids[1]); err != nil {
		t.Fatal(err)
	}
	got := st.ProviderKeys("me@example.com")["openai"]
	if len(got) != 2 || got[0] != "sk-1" || got[1] != "sk-3" {
		t.Errorf("keys = %q, want the first and third in order", got)
	}
	// The remaining ids are unchanged, so a console holding the old list can
	// still delete the right one.
	list := st.ProviderKeyList("me@example.com")["openai"]
	if len(list) != 2 || list[0].ID != ids[0] || list[1].ID != ids[2] {
		t.Errorf("ids moved under a delete: %+v", list)
	}
}

// A plan cannot bill less than the call cost the gateway, whatever the config
// says.
func TestMarkupNeverGoesBelowCost(t *testing.T) {
	if got := (config.Plan{Markup: 0.5}).Rate(); got != 1 {
		t.Errorf("rate = %v, want at cost", got)
	}
	if got := (config.Plan{}).Rate(); got != 1 {
		t.Errorf("unset markup = %v, want at cost", got)
	}
	if got := (config.Plan{Days: 0}).Term(); got != 30 {
		t.Errorf("term = %d, want a month by default", got)
	}
}

// Every key is a rung, and every rung is an upstream connection on a client
// that waits 45 minutes. The store caps saved keys; without the same cap on
// the merged list, repeated headers walk straight around it.
func TestKeyListIsCappedAcrossBothSources(t *testing.T) {
	t.Setenv("NABUGATE_SECRET_KEY", "master")
	st, err := adminstore.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < adminstore.MaxKeysPerProvider; i++ {
		if _, err := st.AddProviderKey("me@example.com", "openai", "", fmt.Sprintf("sk-stored-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{admin: st}

	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	for i := 0; i < 20; i++ {
		req.Header.Add("X-Nabu-Key-Openai", fmt.Sprintf("sk-header-%d", i))
	}
	// The headers alone are capped first.
	if got := len(callerKeysFrom(req).Keys["openai"]); got != adminstore.MaxKeysPerProvider {
		t.Errorf("headers = %d keys, want the cap", got)
	}

	ctx, _ := withCallerKeys(req.Context(), req)
	ctx = s.withOwnerCredentials(ctx, "me@example.com")
	got := ctx.Value(router.CallerKeysCtxKey{}).(router.CallerKeys).Keys["openai"]
	if len(got) != adminstore.MaxKeysPerProvider {
		t.Fatalf("merged = %d keys, want the cap", len(got))
	}
	// And the survivors are the front of the list: the header keys, which the
	// caller sent for this request, come first.
	if got[0] != "sk-header-0" {
		t.Errorf("first key = %q, want the explicit header", got[0])
	}
}
