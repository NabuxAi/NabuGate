package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

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
	// HaveKey and KeyPrefix describe *your* saved credential. The key itself is
	// never in this response — the prefix exists so the console can show which
	// one is saved without being able to show it.
	HaveKey   bool   `json:"have_key"`
	KeyPrefix string `json:"key_prefix,omitempty"`

	// Access is "auto" or "request": whether spending the gateway's own key
	// here needs a human. Grant is your standing on that — "", "pending",
	// "approved" or "denied".
	Access string `json:"access"`
	Grant  string `json:"grant,omitempty"`
	// UsesGatewayKey says the plain thing the two fields above imply: right
	// now, can you route to this provider on the gateway's credential.
	UsesGatewayKey bool `json:"uses_gateway_key"`
}

// catalogue assembles the rows for one signed-in user.
func (s *Server) catalogueFor(email string) []providerView {
	live := map[string]bool{}
	for _, name := range s.router.ProviderNames() {
		live[name] = true
	}
	saved := map[string]string{}
	grants := map[string]string{}
	if s.admin != nil && email != "" {
		saved = s.admin.ProviderKeyPrefixes(email)
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
		if prefix, ok := saved[name]; ok {
			row.HaveKey, row.KeyPrefix = true, prefix
		}
		row.UsesGatewayKey = row.Live &&
			(row.Access == "auto" || grants[name] == adminstore.StatusApproved)
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
	writeJSON(w, http.StatusOK, map[string]any{
		"providers": s.catalogueFor(email),
		// Whether this deployment can store a key at all. Without it the
		// console offers the header instead of a form that would fail.
		"can_store_keys": s.admin != nil && s.admin.SecretConfigured(),
	})
}

// saveProviderKey stores the caller's own upstream credential, sealed.
func (s *Server) saveProviderKey(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	name := strings.ToLower(r.PathValue("name"))

	var body struct {
		Key string `json:"key"`
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
	if err := s.admin.SetProviderKey(email, name, body.Key); err != nil {
		if err == adminstore.ErrNoSecret {
			// Storing it in the clear is the one thing worse than not storing
			// it, so say what is missing rather than degrading.
			writeError(w, http.StatusServiceUnavailable, "this gateway has no NABUGATE_SECRET_KEY set, so it will not store keys. Send yours per request with the X-Nabu-Key-"+name+" header instead.")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.log.Info("provider key saved", "owner", email, "provider", name)
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "provider": name})
}

func (s *Server) deleteProviderKey(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	name := strings.ToLower(r.PathValue("name"))
	if err := s.admin.DeleteProviderKey(email, name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "provider": name})
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
