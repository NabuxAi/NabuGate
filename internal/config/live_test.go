package config

import (
	"strings"
	"testing"

	"nabugate/internal/usage"
)

func liveConfig(prices map[string]usage.Price) *Config {
	return &Config{
		Live:    map[string]ModelRoute{"nabu-live": {Primary: Target{Provider: "openai", Model: "gpt-live-1"}}},
		Pricing: prices,
	}
}

func TestAnUnpricedLiveAliasIsRefused(t *testing.T) {
	problems := liveConfig(map[string]usage.Price{"openai/gpt-4o": {Input: 1, Output: 2}}).LiveProblems()
	if !strings.Contains(problems["nabu-live"], "no per_minute price for openai/gpt-live-1") {
		t.Fatalf("problems = %v", problems)
	}
}

func TestAPricedLiveAliasIsServed(t *testing.T) {
	if problems := liveConfig(map[string]usage.Price{"openai/gpt-live-1": {PerMinute: 0.05}}).LiveProblems(); len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
}

func TestATokenPriceIsNotAMinutePrice(t *testing.T) {
	// A live minute is billed from per_minute alone; input/output would price
	// every call at nothing.
	problems := liveConfig(map[string]usage.Price{"openai/gpt-live-1": {Input: 5, Output: 20}}).LiveProblems()
	if _, refused := problems["nabu-live"]; !refused {
		t.Fatal("a live alias priced per token was accepted")
	}
}

func TestLiveProblemsFollowTheModelRegistry(t *testing.T) {
	cfg := &Config{
		Live: map[string]ModelRoute{"nabu-live": {Primary: Target{Model: "gpt-live-1"}}},
		Registry: map[string]ModelEntry{"gpt-live-1": {Serves: []Serving{
			{Provider: "openai", Model: "gpt-live-1"},
			{Provider: "azure", Model: "gpt-live-1-eu"},
		}}},
		Pricing: map[string]usage.Price{"openai/gpt-live-1": {PerMinute: 0.05}},
	}
	problems := cfg.LiveProblems()
	if !strings.Contains(problems["nabu-live"], "azure/gpt-live-1-eu") || strings.Contains(problems["nabu-live"], "openai/") {
		t.Fatalf("problems = %v", problems)
	}
}

func TestLiveProblemsNameAModelNobodyServes(t *testing.T) {
	cfg := &Config{Live: map[string]ModelRoute{"nabu-live": {Primary: Target{Model: "ghost"}}}}
	if !strings.Contains(cfg.LiveProblems()["nabu-live"], "no provider serves ghost") {
		t.Fatalf("problems = %v", cfg.LiveProblems())
	}
}

func TestShippedConfigsPriceEveryLiveAlias(t *testing.T) {
	// The image bakes config.default.yaml; config.example.yaml is what an
	// operator copies. An unpriced live alias in either is refused at start-up,
	// which would take voice down on the next deploy.
	for _, path := range []string{"../../config.default.yaml", "../../config.example.yaml"} {
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if len(cfg.Live) == 0 {
			t.Fatalf("%s declares no live alias", path)
		}
		if problems := cfg.LiveProblems(); len(problems) != 0 {
			t.Fatalf("%s: %v", path, problems)
		}
	}
}
