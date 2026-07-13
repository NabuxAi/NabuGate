package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func containsSubstr(haystack []string, needle string) bool {
	for _, value := range haystack {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
