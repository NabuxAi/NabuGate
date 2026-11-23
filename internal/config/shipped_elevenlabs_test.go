package config

import (
	"testing"

	"nabugate/internal/policy"
)

// NabuGen's voice studio reaches ElevenLabs only through /elevenlabs/*, and
// that route checks the project key's allow-list for the "elevenlabs" alias.
// Without it every voice call answered 403 "alias \"elevenlabs\" is not
// permitted for this key", and the panel told users no NabuGate key was set.
func TestShippedNabuGenKeyMayUseTheElevenLabsProxy(t *testing.T) {
	cfg, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, k := range cfg.Server.Keys {
		if k.Project != "nabugen" {
			continue
		}
		if !(policy.Policy{Allow: k.Allow}).Allows("elevenlabs") {
			t.Fatalf("nabugen key allow-list %v does not permit the elevenlabs proxy", k.Allow)
		}
		return
	}
	t.Fatal("config.default.yaml has no nabugen key")
}
