package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode"

	"nabugate/internal/adminstore"
	"nabugate/internal/config"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// The console's view of every alias: what serves it, in which order, at what
// price, and — for a speech or live alias — in which voice. An administrator
// can change which configured rung answers first and the default voice; both
// apply from the next request, without a deploy.

// maxVoiceName bounds a voice name: a vendor voice id is a few dozen characters,
// and anything longer is not one.
const maxVoiceName = 64

type aliasCoordinate struct {
	Provider string      `json:"provider"`
	Model    string      `json:"model"`
	Price    usage.Price `json:"price"`
	// Priced is false when nothing in the price table covers this coordinate:
	// a call served here is metered at nothing.
	Priced bool `json:"priced"`
}

type aliasRung struct {
	Key         string            `json:"key"`
	Coordinates []aliasCoordinate `json:"coordinates"`
}

type aliasView struct {
	Alias string `json:"alias"`
	Kind  string `json:"kind"`
	// Rungs are in the order they are tried: the chosen primary first.
	Rungs []aliasRung `json:"rungs"`
	// Primary is the rung that answers first; ConfigPrimary is the one the
	// config puts first, so the console can tell a changed alias from one that
	// is as shipped.
	Primary       string `json:"primary"`
	ConfigPrimary string `json:"config_primary"`
	// Voice is the default voice in effect; ConfigVoice is the config's.
	Voice       string `json:"voice,omitempty"`
	ConfigVoice string `json:"config_voice,omitempty"`
	// Voiced is true for the kinds a voice means anything to.
	Voiced    bool      `json:"voiced"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

func voicedKind(kind string) bool { return kind == "audio" || kind == "live" }

// applyAliasSettings hands the console's saved choices to the router. Called
// when the store is attached and after every change, so a choice routes the
// next request.
func (s *Server) applyAliasSettings() {
	if s.admin == nil || s.router == nil {
		return
	}
	saved := s.admin.AliasSettings()
	settings := make(map[string]router.AliasSetting, len(saved))
	for alias, st := range saved {
		settings[alias] = router.AliasSetting{Primary: st.Primary, Voice: st.Voice}
	}
	s.router.SetAliasSettings(settings)
}

func (s *Server) aliasView(info router.RouteInfo, saved adminstore.AliasSetting) aliasView {
	v := aliasView{
		Alias:       info.Alias,
		Kind:        info.Kind,
		ConfigVoice: info.Voice,
		Voice:       info.Voice,
		Voiced:      voicedKind(info.Kind),
		UpdatedBy:   saved.UpdatedBy,
		UpdatedAt:   saved.UpdatedAt,
	}
	if info.Setting.Voice != "" {
		v.Voice = info.Setting.Voice
	}
	rungs := make([]aliasRung, 0, len(info.Rungs))
	first := -1
	for i, rung := range info.Rungs {
		out := aliasRung{Key: rung.Key, Coordinates: []aliasCoordinate{}}
		for _, c := range rung.Coordinates {
			price, priced := s.usage.Price(c.Provider, c.Model)
			out.Coordinates = append(out.Coordinates, aliasCoordinate{Provider: c.Provider, Model: c.Model, Price: price, Priced: priced})
		}
		rungs = append(rungs, out)
		if rung.Key == info.Setting.Primary {
			first = i
		}
	}
	if len(rungs) > 0 {
		v.ConfigPrimary = rungs[0].Key
		v.Primary = rungs[0].Key
	}
	// The same reordering the router applies: the chosen rung first, the rest
	// behind it in config order. A choice naming no rung any more is ignored
	// there, and shown as ignored here.
	if first > 0 {
		chosen := rungs[first]
		rungs = append(append([]aliasRung{chosen}, rungs[:first]...), rungs[first+1:]...)
		v.Primary = chosen.Key
	}
	v.Rungs = rungs
	return v
}

// listAliases — GET /api/aliases. Every signed-in user may read the routing and
// its prices, because what a model costs is what their balance pays; only an
// administrator may change it (saveAliasSetting).
func (s *Server) listAliases(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value(consoleAdminCtxKey{}).(bool)
	saved := s.admin.AliasSettings()
	views := make([]aliasView, 0)
	for _, info := range s.router.Routes() {
		views = append(views, s.aliasView(info, saved[info.Alias]))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"aliases":  views,
		"voices":   s.router.VoiceNames(),
		"can_edit": isAdmin,
	})
}

type aliasSettingBody struct {
	Primary string `json:"primary"`
	Voice   string `json:"voice"`
}

// saveAliasSetting — PUT /api/aliases/{alias}. Empty fields put the alias back
// to the config's choice.
//
// The primary has to be one of the alias's own configured rungs: the console
// reorders what the config routes to and never adds a destination, so a choice
// here cannot send a call to a model nobody priced or a provider nobody keyed.
func (s *Server) saveAliasSetting(w http.ResponseWriter, r *http.Request) {
	alias := strings.TrimSpace(r.PathValue("alias"))
	kind, route := s.router.RouteKind(alias)
	if kind == "" {
		writeError(w, http.StatusNotFound, "unknown alias")
		return
	}
	var body aliasSettingBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.Primary = strings.TrimSpace(body.Primary)
	body.Voice = strings.TrimSpace(body.Voice)

	if body.Primary != "" {
		known := false
		for _, t := range append([]config.Target{route.Primary}, route.Fallback...) {
			if router.RungKey(t) == body.Primary {
				known = true
				break
			}
		}
		if !known {
			writeError(w, http.StatusBadRequest, "primary must be one of this alias's configured models")
			return
		}
		// The config's own first rung is no choice at all; store nothing for it.
		if body.Primary == router.RungKey(route.Primary) {
			body.Primary = ""
		}
	}
	if body.Voice != "" {
		if !voicedKind(kind) {
			writeError(w, http.StatusBadRequest, "only a speech or live alias has a voice")
			return
		}
		if !validVoiceName(body.Voice) {
			writeError(w, http.StatusBadRequest, "voice must be a name of at most 64 printable characters")
			return
		}
		if body.Voice == route.Voice {
			body.Voice = ""
		}
	}

	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	setting := adminstore.AliasSetting{
		Primary:   body.Primary,
		Voice:     body.Voice,
		UpdatedBy: email,
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.admin.SetAliasSetting(alias, setting); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.applyAliasSettings()
	s.log.Info("alias setting changed", "alias", alias, "primary", setting.Primary, "voice", setting.Voice, "by", email)

	for _, info := range s.router.Routes() {
		if info.Alias == alias {
			writeJSON(w, http.StatusOK, s.aliasView(info, s.admin.AliasSettings()[alias]))
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func validVoiceName(v string) bool {
	if len(v) > maxVoiceName {
		return false
	}
	for _, c := range v {
		if !unicode.IsPrint(c) {
			return false
		}
	}
	return true
}
