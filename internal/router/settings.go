package router

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"nabugate/internal/config"
)

// AliasSetting is what an administrator chose for one alias in the console:
// which of its configured rungs answers first, and the voice a speech or live
// alias speaks in when the caller names none. Both apply from the next request
// on, without a deploy. An empty field leaves the config's choice in place.
type AliasSetting struct {
	Primary string `json:"primary,omitempty"`
	Voice   string `json:"voice,omitempty"`
}

// RungKey names one configured rung of an alias: "provider/model", or the bare
// model for a rung the model registry expands across providers.
func RungKey(t config.Target) string {
	if t.Provider == "" {
		return t.Model
	}
	return t.Provider + "/" + t.Model
}

// SetVoices installs the named voices from the config's `voices:` block.
func (r *Router) SetVoices(voices map[string]map[string]string) {
	r.voices = voices
}

// SetAliasSettings replaces every alias's console setting at once.
func (r *Router) SetAliasSettings(settings map[string]AliasSetting) {
	copied := make(map[string]AliasSetting, len(settings))
	for alias, s := range settings {
		copied[alias] = s
	}
	r.settingsMu.Lock()
	r.settings = copied
	r.settingsMu.Unlock()
}

func (r *Router) setting(alias string) AliasSetting {
	r.settingsMu.RLock()
	defer r.settingsMu.RUnlock()
	return r.settings[alias]
}

// rungs is the order an alias's rungs are tried in: the config's, with the rung
// an administrator chose moved to the front and the rest kept behind it in
// their configured order as its fallbacks.
//
// A choice naming no rung of this alias any more — the config changed under
// it — is ignored rather than followed, so a stale setting can only ever
// reorder what the config routes to, never add a destination of its own.
func (r *Router) rungs(alias string, route config.ModelRoute) []config.Target {
	chain := append([]config.Target{route.Primary}, route.Fallback...)
	chosen := r.setting(alias).Primary
	if chosen == "" {
		return chain
	}
	for i, t := range chain {
		if RungKey(t) != chosen {
			continue
		}
		if i == 0 {
			return chain
		}
		ordered := make([]config.Target, 0, len(chain))
		ordered = append(ordered, t)
		ordered = append(ordered, chain[:i]...)
		return append(ordered, chain[i+1:]...)
	}
	return chain
}

// voiceFor is the voice to send one provider for the voice a call asked for.
//
// A name from `voices:` becomes that provider's own voice. When the name has no
// voice for this provider, nothing is sent and the vendor speaks in its own
// default: a fallback rung answering in a different voice is a smaller failure
// than a fallback refused because the first vendor's voice name meant nothing
// to it. Anything not named in `voices:` is a vendor voice the caller chose and
// travels untouched, as it always has.
func (r *Router) voiceFor(voice, prov string) string {
	byProvider, named := r.voices[voice]
	if !named {
		return voice
	}
	return byProvider[prov]
}

// requestedVoice is the voice a call speaks in: the one the caller named, else
// the alias's default — the administrator's choice, else the config's.
func (r *Router) requestedVoice(alias string, route config.ModelRoute, asked string) string {
	if asked != "" {
		return asked
	}
	if v := r.setting(alias).Voice; v != "" {
		return v
	}
	return route.Voice
}

// VoiceNames are the names declared in `voices:`, sorted, with the providers
// each one speaks for — what the console offers as an alias's default voice.
func (r *Router) VoiceNames() map[string][]string {
	out := make(map[string][]string, len(r.voices))
	for name, byProvider := range r.voices {
		provs := make([]string, 0, len(byProvider))
		for p := range byProvider {
			provs = append(provs, p)
		}
		sort.Strings(provs)
		out[name] = provs
	}
	return out
}

// RouteInfo is one alias as the console shows and edits it.
type RouteInfo struct {
	Alias string
	// Kind is the config section the alias is declared in: models, images,
	// audio, transcription, embeddings, decisions or live.
	Kind string
	// Rungs are the configured rungs in config order, each with the concrete
	// provider coordinates it expands to on this gateway (a rung naming a
	// provider is its own coordinate when that provider has a key here).
	Rungs   []RungInfo
	Voice   string
	Setting AliasSetting
}

// RungInfo is one configured rung and what it can actually reach.
type RungInfo struct {
	Key         string
	Coordinates []config.Target
}

// Routes lists every configured alias with its rungs and settings, sorted by
// kind and then by name.
func (r *Router) Routes() []RouteInfo {
	sections := []struct {
		kind   string
		routes map[string]config.ModelRoute
	}{
		{"models", r.models}, {"images", r.images}, {"audio", r.audio},
		{"transcription", r.transcription}, {"embeddings", r.embeddings},
		{"decisions", r.decisions}, {"live", r.live},
	}
	var out []RouteInfo
	for _, sec := range sections {
		names := make([]string, 0, len(sec.routes))
		for alias := range sec.routes {
			names = append(names, alias)
		}
		sort.Strings(names)
		for _, alias := range names {
			route := sec.routes[alias]
			info := RouteInfo{Alias: alias, Kind: sec.kind, Voice: route.Voice, Setting: r.setting(alias)}
			for _, t := range append([]config.Target{route.Primary}, route.Fallback...) {
				rung := RungInfo{Key: RungKey(t)}
				for _, c := range r.expand(t) {
					if _, live := r.adapters[c.Provider]; live {
						rung.Coordinates = append(rung.Coordinates, c)
					}
				}
				info.Rungs = append(info.Rungs, rung)
			}
			out = append(out, info)
		}
	}
	return out
}

// RouteKind is the config section an alias is declared in, or "" for none.
func (r *Router) RouteKind(alias string) (string, config.ModelRoute) {
	for _, sec := range []struct {
		kind   string
		routes map[string]config.ModelRoute
	}{
		{"models", r.models}, {"images", r.images}, {"audio", r.audio},
		{"transcription", r.transcription}, {"embeddings", r.embeddings},
		{"decisions", r.decisions}, {"live", r.live},
	} {
		if route, ok := sec.routes[alias]; ok {
			return sec.kind, route
		}
	}
	return "", config.ModelRoute{}
}

// liveVoice reads the voice a Realtime session body asks for, at
// session.audio.output.voice. Absent or unreadable is "".
func liveVoice(body []byte) string {
	var shape struct {
		Session struct {
			Audio struct {
				Output struct {
					Voice string `json:"voice"`
				} `json:"output"`
			} `json:"audio"`
		} `json:"session"`
	}
	if json.Unmarshal(body, &shape) != nil {
		return ""
	}
	return strings.TrimSpace(shape.Session.Audio.Output.Voice)
}

// withLiveVoice sets session.audio.output.voice, or removes it for "", keeping
// every other field of the body exactly as the caller sent it.
func withLiveVoice(body []byte, voice string) ([]byte, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, err
	}
	session := map[string]json.RawMessage{}
	if raw, ok := top["session"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if err := json.Unmarshal(raw, &session); err != nil {
			return nil, err
		}
	}
	audio := map[string]json.RawMessage{}
	if raw, ok := session["audio"]; ok {
		if err := json.Unmarshal(raw, &audio); err != nil {
			return nil, err
		}
	}
	output := map[string]json.RawMessage{}
	if raw, ok := audio["output"]; ok {
		if err := json.Unmarshal(raw, &output); err != nil {
			return nil, err
		}
	}
	if voice == "" {
		if _, had := output["voice"]; !had {
			return body, nil
		}
		delete(output, "voice")
	} else {
		v, _ := json.Marshal(voice)
		output["voice"] = v
	}
	var err error
	if audio["output"], err = json.Marshal(output); err != nil {
		return nil, err
	}
	if session["audio"], err = json.Marshal(audio); err != nil {
		return nil, err
	}
	if top["session"], err = json.Marshal(session); err != nil {
		return nil, err
	}
	return json.Marshal(top)
}
