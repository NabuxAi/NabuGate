package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nabugate/internal/provider"
)

const miniConfig = `
server:
  api_keys: ["${NABU_ADMIN_KEY}"]
providers:
  openai:
    enabled: true
    type: openai
    base_url: "https://api.openai.com/v1"
    api_key_env: "OPENAI_API_KEY"
`

func TestParseExpandsEnvAndDefaultsPort(t *testing.T) {
	t.Setenv("NABU_ADMIN_KEY", "secret-admin")

	cfg, err := Parse(miniConfig)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("default port = %d, want 8080", cfg.Server.Port)
	}
	if got := cfg.Server.APIKeys; len(got) != 1 || got[0] != "secret-admin" {
		t.Errorf("api_keys = %v, want [secret-admin]", got)
	}
}

func TestResolvePrefersInlineEnv(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("server:\n  port: 1111\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvConfigYAML, "server:\n  port: 2222\n")

	cfg, err := Resolve(configPath)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.Server.Port != 2222 {
		t.Errorf("port = %d, want 2222", cfg.Server.Port)
	}
}

func TestResolveFallsBackToFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("server:\n  port: 3333\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvConfigYAML, "   \n")

	cfg, err := Resolve(configPath)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.Server.Port != 3333 {
		t.Errorf("port = %d, want 3333", cfg.Server.Port)
	}
}

func TestLoadMissingFileError(t *testing.T) {
	t.Setenv(EnvConfigYAML, "")

	_, err := Resolve(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected an error for a missing config file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should mention not found", err)
	}
	if !strings.Contains(err.Error(), EnvConfigYAML) {
		t.Errorf("error %q should mention %s", err, EnvConfigYAML)
	}
}

func TestDefaultConfigParses(t *testing.T) {
	t.Setenv("NABU_API_KEY", "admin-from-env")

	raw, err := os.ReadFile("../../config.default.yaml")
	if err != nil {
		t.Fatalf("read config.default.yaml: %v", err)
	}
	cfg, err := Parse(string(raw))
	if err != nil {
		t.Fatalf("parse config.default.yaml: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("port = %d, want 8080", cfg.Server.Port)
	}
	if len(cfg.Server.APIKeys) != 1 || cfg.Server.APIKeys[0] != "admin-from-env" {
		t.Errorf("api_keys = %v, want [admin-from-env]", cfg.Server.APIKeys)
	}

	for _, name := range []string{"dahl", "parspack", "ollama"} {
		if _, ok := cfg.Providers[name]; !ok {
			t.Errorf("default config should define provider %q", name)
		}
	}
	for _, alias := range []string{"nabu-fast", "nabu-local", "nabu-parspack"} {
		if _, ok := cfg.Models[alias]; !ok {
			t.Errorf("default config should define alias %q", alias)
		}
	}

	local := cfg.Models["nabu-local"]
	if local.Primary.Provider != "ollama" {
		t.Errorf("nabu-local primary provider = %q, want ollama", local.Primary.Provider)
	}
	if len(local.Fallback) != 0 {
		t.Errorf("nabu-local should have no fallback, got %d", len(local.Fallback))
	}

	parspack := cfg.Providers["parspack"]
	if parspack.Type != "openai" || parspack.APIKeyEnv != "PARSPACK_API_KEY" {
		t.Errorf("parspack provider = %+v", parspack)
	}
	parspackRoute := cfg.Models["nabu-parspack"]
	if parspackRoute.Primary.Provider != "parspack" {
		t.Errorf("nabu-parspack primary provider = %q, want parspack", parspackRoute.Primary.Provider)
	}

	t.Setenv("PARSPACK_API_KEY", "pk-test")
	adapters, _ := cfg.BuildAdapters()
	if _, ok := adapters["parspack"]; !ok {
		t.Error("BuildAdapters should build parspack when its key is set")
	}
}

func TestBuildAdaptersKeylessProvider(t *testing.T) {
	withBase := &Config{Providers: map[string]ProviderConfig{
		"ollama": {
			Enabled: true,
			Type:    "openai",
			BaseURL: "http://ollama:11434/v1",
		},
	}}
	adapters, _ := withBase.BuildAdapters()
	if _, ok := adapters["ollama"]; !ok {
		t.Error("keyless ollama provider with base_url should be built")
	}

	withoutBase := &Config{Providers: map[string]ProviderConfig{
		"ollama": {
			Enabled: true,
			Type:    "openai",
		},
	}}
	adapters, warnings := withoutBase.BuildAdapters()
	if _, ok := adapters["ollama"]; ok {
		t.Error("keyless ollama provider without base_url should be skipped")
	}
	if !containsSubstr(warnings, "base_url") {
		t.Errorf("expected a base_url warning, got %v", warnings)
	}

	badType := &Config{Providers: map[string]ProviderConfig{
		"claude": {
			Enabled: true,
			Type:    "anthropic",
			BaseURL: "https://api.anthropic.com/v1",
		},
	}}
	adapters, warnings = badType.BuildAdapters()
	if _, ok := adapters["claude"]; ok {
		t.Error("keyless anthropic provider should be skipped")
	}
	if !containsSubstr(warnings, "api_key_env") {
		t.Errorf("expected an api_key_env warning, got %v", warnings)
	}
}

func TestBuildAdaptersPexels(t *testing.T) {
	cfg := &Config{Providers: map[string]ProviderConfig{
		"pexels": {
			Enabled:   true,
			Type:      "pexels",
			BaseURL:   "https://api.pexels.com/v1",
			APIKeyEnv: "PEXELS_API_KEY",
		},
	}}

	// No key → skipped with a warning (like any other keyed provider).
	adapters, warnings := cfg.BuildAdapters()
	if _, ok := adapters["pexels"]; ok {
		t.Error("pexels should be skipped when PEXELS_API_KEY is empty")
	}
	if !containsSubstr(warnings, "PEXELS_API_KEY") {
		t.Errorf("expected a missing-key warning, got %v", warnings)
	}

	// Key set → built.
	t.Setenv("PEXELS_API_KEY", "pk-test")
	adapters, _ = cfg.BuildAdapters()
	ad, ok := adapters["pexels"]
	if !ok {
		t.Fatal("pexels adapter should be built when its key is set")
	}
	if _, ok := ad.(provider.ImageAdapter); !ok {
		t.Error("pexels adapter should implement ImageAdapter")
	}
}

func TestLoadDirectoryError(t *testing.T) {
	t.Setenv(EnvConfigYAML, "")

	dir := t.TempDir()
	mount := filepath.Join(dir, "config.yaml")
	if err := os.Mkdir(mount, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve(mount)
	if err == nil {
		t.Fatal("expected an error for a directory config path")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("error %q should mention directory", err)
	}
	if !strings.Contains(err.Error(), EnvConfigYAML) {
		t.Errorf("error %q should mention %s", err, EnvConfigYAML)
	}
}

func TestBuildAgentsInline(t *testing.T) {
	const raw = `
agents:
  cine-writer:
    model: nabu-fast
    system: "You write."
    temperature: 0.4
  cine-director:
    name: cine-creative-director
    model: nabu-smart
    system: "You direct."
  broken:
    system: "no model here"
`
	cfg, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	reg, warnings := cfg.BuildAgents()

	if _, ok := reg.Lookup("cine-writer"); !ok {
		t.Error("cine-writer should be registered from its map key")
	}
	// The `name:` override wins over the map key.
	if _, ok := reg.Lookup("cine-creative-director"); !ok {
		t.Error("cine-creative-director (name override) should be registered")
	}
	if _, ok := reg.Lookup("cine-director"); ok {
		t.Error("map key should not register when name overrides it")
	}
	// The model-less entry is skipped with a warning, not fatal.
	if _, ok := reg.Lookup("broken"); ok {
		t.Error("agent without a model should be skipped")
	}
	if !containsSubstr(warnings, "broken") {
		t.Errorf("expected a warning about the broken agent, got %v", warnings)
	}
	w, _ := reg.Lookup("cine-writer")
	if w.Temperature == nil || *w.Temperature != 0.4 {
		t.Errorf("cine-writer temperature = %v, want 0.4", w.Temperature)
	}
}

func TestBuildAgentsFromDir(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"motion.yaml": "name: cine-motion-designer\nmodel: nabu-smart\nsystem: |\n  You choreograph motion.\n",
		"perf.yml":    "model: nabu-fast\nsystem: You optimise.\n", // name falls back to file base
		"notes.txt":   "model: ignored\n",                          // non-YAML, ignored
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &Config{AgentsDir: dir}
	reg, warnings := cfg.BuildAgents()
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if _, ok := reg.Lookup("cine-motion-designer"); !ok {
		t.Error("cine-motion-designer should load from motion.yaml's name field")
	}
	if _, ok := reg.Lookup("perf"); !ok {
		t.Error("perf should load with name derived from perf.yml file base")
	}
	if reg.Len() != 2 {
		t.Errorf("registry size = %d, want 2 (notes.txt ignored)", reg.Len())
	}
}

func TestShippedCinematicAgentsLoad(t *testing.T) {
	cfg := &Config{AgentsDir: "../../agents"}
	reg, warnings := cfg.BuildAgents()
	if len(warnings) != 0 {
		t.Errorf("shipped agents produced warnings: %v", warnings)
	}
	for _, name := range []string{
		"cine-creative-director",
		"cine-interactive-designer",
		"cine-motion-designer",
		"cine-3d-artist",
		"cine-frontend-developer",
		"cine-content-strategist",
		"cine-performance-a11y",
	} {
		ag, ok := reg.Lookup(name)
		if !ok {
			t.Errorf("shipped agent %q should load from ./agents", name)
			continue
		}
		if strings.TrimSpace(ag.System) == "" {
			t.Errorf("agent %q has an empty system prompt", name)
		}
		if strings.TrimSpace(ag.Model) == "" {
			t.Errorf("agent %q has no model", name)
		}
	}
}

func containsSubstr(haystack []string, needle string) bool {
	for _, value := range haystack {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
