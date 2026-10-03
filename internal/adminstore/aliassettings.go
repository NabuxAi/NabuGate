package adminstore

import (
	"strings"
	"time"
)

// AliasSetting is what an administrator chose for one alias in the console:
// the configured rung that answers first, and the voice a speech or live alias
// speaks in when the caller names none. Empty fields leave the config's own
// choice in place, which is how an alias is put back.
//
// Kept here rather than in the config because it changes without a deploy:
// switching nabu-live between gpt-realtime and gpt-realtime-mini took two
// commits and two deploys on 26 September 2026, and each one cut every call in
// progress.
type AliasSetting struct {
	Primary   string    `json:"primary,omitempty"`
	Voice     string    `json:"voice,omitempty"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// AliasSettings returns a copy of every alias's console setting.
func (s *Store) AliasSettings() map[string]AliasSetting {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]AliasSetting, len(s.st.AliasSettings))
	for alias, setting := range s.st.AliasSettings {
		out[alias] = setting
	}
	return out
}

// SetAliasSetting saves one alias's setting and writes it to disk at once — a
// routing choice lost to a crash would quietly route back. A setting with
// neither a primary nor a voice is removed, putting the alias back to the
// config's choice.
func (s *Store) SetAliasSetting(alias string, setting AliasSetting) error {
	alias = strings.TrimSpace(alias)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st.AliasSettings == nil {
		s.st.AliasSettings = map[string]AliasSetting{}
	}
	if setting.Primary == "" && setting.Voice == "" {
		delete(s.st.AliasSettings, alias)
	} else {
		s.st.AliasSettings[alias] = setting
	}
	return s.save()
}
