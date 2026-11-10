package config

import (
	"os"
	"strings"
	"testing"

	"nabugate/internal/provider"
)

func TestShippedConfigsEnableRouterahModelDiscoveryWithKey(t *testing.T) {
	t.Setenv("ROUTERAH_API_KEY", "routerah-test-key")
	for _, path := range []string{"../../config.example.yaml", "../../config.default.yaml"} {
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		p, ok := cfg.Providers["routerah"]
		if !ok || !p.Enabled || !p.Passthrough || p.Type != "openai" {
			t.Errorf("%s: missing usable Routerah config: %+v", path, p)
		}
		if p.BaseURL != "https://api.routerah.app/v1" ||
			p.ModelsURL != "https://api.routerah.app/api/pricing/catalog" ||
			p.APIKeyEnv != "ROUTERAH_API_KEY" {
			t.Errorf("%s: incorrect Routerah endpoint/key config: %+v", path, p)
		}
		adapters, warnings := cfg.BuildAdapters()
		if _, ok := adapters["routerah"].(*provider.OpenAIAdapter); !ok {
			t.Errorf("%s: Routerah adapter not built; warnings: %v", path, warnings)
		}
		if _, ok := cfg.Passthroughs(adapters)["routerah"]; !ok {
			t.Errorf("%s: Routerah not available for passthrough", path)
		}
		if _, ok := cfg.CallerAdapter("routerah", "another-key"); !ok {
			t.Errorf("%s: caller-provided Routerah key cannot be used", path)
		}
	}
}

func TestRouterahDisabledWithoutConfiguredKey(t *testing.T) {
	t.Setenv("ROUTERAH_API_KEY", "")
	cfg, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	adapters, _ := cfg.BuildAdapters()
	if _, ok := adapters["routerah"]; ok {
		t.Fatal("Routerah should not spend an absent key")
	}
	if _, ok := cfg.Passthroughs(adapters)["routerah"]; ok {
		t.Fatal("unconfigured Routerah models should not be advertised as callable")
	}
}

func TestComposeFilesPassRouterahKey(t *testing.T) {
	for _, path := range []string{"../../docker-compose.yml", "../../docker-compose.coolify.yml"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "ROUTERAH_API_KEY=${ROUTERAH_API_KEY:-}") {
			t.Errorf("%s omits Routerah from container env", path)
		}
	}
}
