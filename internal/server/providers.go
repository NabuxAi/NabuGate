package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"nabugate/internal/adminstore"
	"nabugate/internal/config"
	"nabugate/internal/vendors"
)

// --- The provider catalogue ---
//
// One screen answering, per upstream: what is it, is it wired up here, does the
// gateway hold a key for it, do I have one of my own saved, and if I want to
// spend the gateway's key on it, may I.
//
// It lists providers this deployment does not have, too. Those are the ones a
// user can ask for, and putting them on the same screen means "why isn't X
// here?" has a visible answer instead of living in someone's head.

// providerView is one row of the catalogue.
type providerView struct {
	vendors.Vendor

	// Configured: this deployment defines it. Live: its adapter actually came
	// up, which is Configured plus a key that is set. The two differ constantly
	// — most of this gateway's providers are configured and keyless.
	Configured  bool   `json:"configured"`
	Live        bool   `json:"live"`
	Passthrough bool   `json:"passthrough"`
	KeyEnv      string `json:"key_env,omitempty"`

	// BYOK: you may send or save your own key for it.
	BYOK bool `json:"byok"`
	// Keys describes *your* saved credentials for this provider, in the order
	// they will be tried: the second is used when the first fails. The keys
	// themselves are never in this response — each row carries a prefix so the
	// console can show which one it is without being able to show it.
	Keys []keyView `json:"keys,omitempty"`
	// HaveKey is len(Keys) > 0, stated plainly because most of the console only
	// asks that.
	HaveKey bool `json:"have_key"`

	// Access is "auto" or "request": whether spending the gateway's own key
	// here needs a human. Grant is your standing on that — "", "pending",
	// "approved" or "denied".
	Access string `json:"access"`
	Grant  string `json:"grant,omitempty"`
	// PlanCovers says your subscription unlocks the gateway's key here, which
	// is the other way past `access: request` besides an admin's approval. It
	// is only ever true for a provider that is actually live.
	PlanCovers bool `json:"plan_covers,omitempty"`
	// UsesGatewayKey says the plain thing the two fields above imply: right
	// now, can you route to this provider on the gateway's credential.
	UsesGatewayKey bool `json:"uses_gateway_key"`
}

// keyView is one saved credential as the console may see it.
//
// A distinct type from adminstore.StoredKey, with no field for the sealed
// material at all, rather than the same record with the blob blanked. Blanking
// relies on every construction site remembering; this way the response could
// not carry the ciphertext even if StoredKey grows another secret field
// tomorrow.
type keyView struct {
	ID      string    `json:"id"`
	Prefix  string    `json:"prefix"`
	Label   string    `json:"label,omitempty"`
	AddedAt time.Time `json:"added_at,omitempty"`
}

func keyViews(keys []adminstore.StoredKey) []keyView {
	out := make([]keyView, 0, len(keys))
	for _, k := range keys {
		out = append(out, keyView{ID: k.ID, Prefix: k.Prefix, Label: k.Label, AddedAt: k.AddedAt})
	}
	return out
}

// catalogue assembles the rows for one signed-in user.
func (s *Server) catalogueFor(email string) []providerView {
	live := map[string]bool{}
	for _, name := range s.router.ProviderNames() {
		live[name] = true
	}
	saved := map[string][]adminstore.StoredKey{}
	grants := map[string]string{}
	var sub adminstore.Subscription
	subscribed := false
	if s.admin != nil && email != "" {
		saved = s.admin.ProviderKeyList(email)
		sub, subscribed = s.admin.ActiveSubscription(email)
		for _, req := range s.admin.ProviderRequestsFor(email) {
			grants[req.Provider] = req.Status
		}
	}

	// Every name we know about from either side: what the config defines, and
	// what the catalogue knows exists in the world.
	names := map[string]bool{}
	for name := range s.providers {
		names[name] = true
	}
	for _, v := range vendors.Known() {
		names[v.Name] = true
	}

	out := make([]providerView, 0, len(names))
	for name := range names {
		meta, configured := s.providers[name]
		row := providerView{
			Vendor:      vendors.Get(name),
			Configured:  configured && meta.Enabled,
			Live:        live[name],
			Passthrough: meta.Passthrough,
			KeyEnv:      meta.KeyEnv,
			BYOK:        meta.BYOK,
			Access:      meta.Access,
			Grant:       grants[name],
		}
		if !configured {
			// Nothing here routes to it yet, so neither key can be used. The row
			// exists so it can be asked for.
			row.Access = "request"
		}
		if keys := saved[name]; len(keys) > 0 {
			row.Keys, row.HaveKey = keyViews(keys), true
		}
		// Live, because a subscription buys the right to spend a key this
		// gateway holds — it cannot conjure a provider that is not wired up.
		// Saying otherwise put "your plan opens this" directly under "not
		// available on our key" on the same card.
		row.PlanCovers = row.Live && subscribed && sub.Covers(name)
		row.UsesGatewayKey = row.Live &&
			(row.Access == "auto" || grants[name] == adminstore.StatusApproved || row.PlanCovers)
		out = append(out, row)
	}

	// Live first, then configured, then the rest — the order someone scanning
	// for "what can I use right now" reads in.
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Live != b.Live {
			return a.Live
		}
		if a.Configured != b.Configured {
			return a.Configured
		}
		return a.Name < b.Name
	})
	return out
}

func (s *Server) listProviders(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	body := map[string]any{
		"providers": s.catalogueFor(email),
		// Whether this deployment can store a key at all. Without it the
		// console offers the header instead of a form that would fail.
		"can_store_keys": s.admin != nil && s.admin.SecretConfigured(),
		"max_keys":       adminstore.MaxKeysPerProvider,
	}
	if s.admin != nil && email != "" {
		if sub, ok := s.admin.SubscriptionOf(email); ok {
			body["subscription"] = sub
			body["subscribed"] = sub.Active(time.Now().UTC())
		}
	}
	writeJSON(w, http.StatusOK, body)
}

// saveProviderKey adds one of the caller's own upstream credentials, sealed.
// Adds rather than replaces: several keys for one provider are tried in turn,
// so a key that dies costs a fallback and not the request.
func (s *Server) saveProviderKey(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	name := strings.ToLower(r.PathValue("name"))

	var body struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "send {\"key\": \"...\"}")
		return
	}
	meta, ok := s.providers[name]
	if !ok || !meta.BYOK {
		writeError(w, http.StatusBadRequest, "this gateway does not route to that provider, so a key for it would go nowhere")
		return
	}
	rec, err := s.admin.AddProviderKey(email, name, body.Label, body.Key)
	if err != nil {
		if err == adminstore.ErrNoSecret {
			// Storing it in the clear is the one thing worse than not storing
			// it, so say what is missing rather than degrading.
			writeError(w, http.StatusServiceUnavailable, "this gateway has no NABUGATE_SECRET_KEY set, so it will not store keys. Send yours per request with the X-Nabu-Key-"+name+" header instead.")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.log.Info("provider key saved", "owner", email, "provider", name, "id", rec.ID)
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "provider": name, "key": keyViews([]adminstore.StoredKey{rec})[0]})
}

// deleteProviderKey forgets one key, or every key for the provider when no id
// is named — which is what "remove my OpenAI key" means when there is one.
func (s *Server) deleteProviderKey(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	name := strings.ToLower(r.PathValue("name"))
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if err := s.admin.DeleteProviderKey(email, name, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "provider": name, "id": id})
}

// requestProvider asks to spend the gateway's credential on a provider.
func (s *Server) requestProvider(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	name := strings.ToLower(r.PathValue("name"))

	var body struct {
		Note string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	meta, configured := s.providers[name]
	if _, known := vendors.Lookup(name); !known && !configured {
		writeError(w, http.StatusBadRequest, "unknown provider")
		return
	}
	// Auto only applies to a provider that is actually wired up and up. Asking
	// for one that is not is a request for someone to wire it up, which is a
	// person's job by definition.
	auto := configured && meta.Enabled && meta.Access == "auto" && s.router.HasProvider(name)

	req, err := s.admin.RequestProvider(email, name, body.Note, auto, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.log.Info("provider access requested", "owner", email, "provider", name, "status", req.Status, "auto", req.Auto)
	writeJSON(w, http.StatusOK, req)
}

// listProviderRequests is the admin queue.
func (s *Server) listProviderRequests(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"requests": s.admin.AllProviderRequests()})
}

// decideProviderRequest approves or denies one, optionally with credit. Credit
// and approval are one action because they are one decision.
func (s *Server) decideProviderRequest(w http.ResponseWriter, r *http.Request) {
	by, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	var body struct {
		ID       string  `json:"id"`
		Approve  bool    `json:"approve"`
		Credit   float64 `json:"credit_usd"`
		Decision string  `json:"decision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		writeError(w, http.StatusBadRequest, "send {\"id\":..., \"approve\":true|false}")
		return
	}
	req, err := s.admin.DecideProviderRequest(body.ID, body.Approve, body.Credit, body.Decision, by)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	s.log.Info("provider access decided", "id", body.ID, "approve", body.Approve, "credit", body.Credit, "by", by)
	writeJSON(w, http.StatusOK, req)
}

// SetProviderMetas hands the server what the config knows about providers the
// router never sees — the ones with no key, and the access rules.
func (s *Server) SetProviderMetas(m map[string]config.ProviderMeta) { s.providers = m }
