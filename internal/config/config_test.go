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

// TestParseExpandsEnvAndDefaultsPort verifies ${VAR} expansion and the default
// port fallback shared by file and inline loading.
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
		t.Errorf("api_keys = %v, want [secret-admin] (env expansion)", got)
	}
}

// TestResolvePrefersInlineEnv verifies NABU_CONFIG_YAML wins over the file path,
// so an auto-created/stale bind-mount file can't shadow the inline config.
func TestResolvePrefersInlineEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  port: 1111\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvConfigYAML, "server:\n  port: 2222\n")

	cfg, err := Resolve(path)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.Server.Port != 2222 {
		t.Errorf("port = %d, want 2222 (inline env should win)", cfg.Server.Port)
	}
}

// TestResolveFallsBackToFile verifies the file is used when the env var is unset
// or blank (whitespace-only must not be treated as inline config).
func TestResolveFallsBackToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  port: 3333\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvConfigYAML, "   \n")

	cfg, err := Resolve(path)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.Server.Port != 3333 {
		t.Errorf("port = %d, want 3333 (should read the file)", cfg.Server.Port)
	}
}

// TestLoadMissingFileError checks the missing-config error is actionable and
// points at the inline-env escape hatch (not a bare os error).
func TestLoadMissingFileError(t *testing.T) {
	t.Setenv(EnvConfigYAML, "")

	_, err := Resolve(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected an error for a missing config file, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should say the file was not found", err)
	}
	if !strings.Contains(err.Error(), EnvConfigYAML) {
		t.Errorf("error %q should point at the %s escape hatch", err, EnvConfigYAML)
	}
}

// TestDefaultConfigParses guards the secret-free default baked into the image:
// it must be valid YAML, expand ${NABU_API_KEY} into the admin key, and carry
// the provider/model routing (no hardcoded gateway credential shipped).
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
		t.Errorf("api_keys = %v, want [admin-from-env] (env expansion)", cfg.Server.APIKeys)
	}
	if _, ok := cfg.Providers["dahl"]; !ok {
		t.Error("default config should define the dahl provider")
	}
	if _, ok := cfg.Models["nabu-fast"]; !ok {
		t.Error("default config should define the nabu-fast alias")
	}
	// Ollama (local, on-prem) must be routable through the gateway too.
	if _, ok := cfg.Providers["ollama"]; !ok {
		t.Error("default config should define the ollama provider")
	}
	local, ok := cfg.Models["nabu-local"]
	if !ok {
		t.Fatal("default config should define the nabu-local alias")
	}
	if local.Primary.Provider != "ollama" {
		t.Errorf("nabu-local primary provider = %q, want ollama", local.Primary.Provider)
	}
	if len(local.Fallback) != 0 {
		t.Errorf("nabu-local should have no cloud fallback (on-prem), got %d", len(local.Fallback))
	}

	// Parspack provider + alias are wired and the OpenAI-wire adapter builds.
	p, ok := cfg.Providers["parspack"]
	if !ok || p.Type != "openai" || p.APIKeyEnv != "PARSPACK_API_KEY" {
		t.Errorf("parspack provider = %+v, want type=openai api_key_env=PARSPACK_API_KEY", p)
	}
	route, ok := cfg.Models["nabu-parspack"]
	if !ok || route.Primary.Provider != "parspack" {
		t.Errorf("nabu-parspack primary = %+v, want provider=parspack", route.Primary)
	}
	t.Setenv("PARSPACK_API_KEY", "pk-test")
	adapters, _ := cfg.BuildAdapters()
	if _, ok := adapters["parspack"]; !ok {
		t.Error("BuildAdapters should build the parspack adapter when its key is set")
	}
}

// TestBuildAdaptersKeylessProvider verifies the keyless-provider path used by a
// local Ollama endpoint: a type:openai provider with no api_key_env is built
// when it has a base_url (enabling the nabu-local alias) and skipped otherwise.
func TestBuildAdaptersKeylessProvider(t *testing.T) {
	withBase := &Config{Providers: map[string]ProviderConfig{
		"ollama": {Enabled: true, Type: "openai", BaseURL: "http://ollama:11434/v1"},
	}}
	adapters, _ := withBase.BuildAdapters()
	if _, ok := adapters["ollama"]; !ok {
		t.Error("keyless ollama provider with a base_url should be built")
	}

	noBase := &Config{Providers: map[string]ProviderConfig{
		"ollama": {Enabled: true, Type: "openai", BaseURL: ""},
	}}
	adapters, warnings := noBase.BuildAdapters()
	if _, ok := adapters["ollama"]; ok {
		t.Error("keyless ollama provider without a base_url should be skipped")
	}
	if !containsSubstr(warnings, "base_url") {
		t.Errorf("expected a base_url warning, got %v", warnings)
	}

	// A keyless non-OpenAI provider is a misconfiguration, not a local endpoint.
	badType := &Config{Providers: map[string]ProviderConfig{
		"claude": {Enabled: true, Type: "anthropic", BaseURL: "https://api.anthropic.com/v1"},
	}}
	adapters, warnings = badType.BuildAdapters()
	if _, ok := adapters["claude"]; ok {
		t.Error("keyless anthropic provider should be skipped (requires api_key_env)")
	}
	if !containsSubstr(warnings, "api_key_env") {
		t.Errorf("expected an api_key_env warning, got %v", warnings)
	}
}

func containsSubstr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

// TestLoadDirectoryError reproduces the Docker missing-mount case (target is a
// directory) and checks the error explains the inline-env escape hatch.
func TestLoadDirectoryError(t *testing.T) {
	t.Setenv(EnvConfigYAML, "") // ensure the file path is taken

	dir := t.TempDir()
	mount := filepath.Join(dir, "config.yaml")
	if err := os.Mkdir(mount, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve(mount)
	if err == nil {
		t.Fatal("expected an error for a directory config path, got nil")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("error %q should mention it is a directory", err)
	}
	if !strings.Contains(err.Error(), EnvConfigYAML) {
		t.Errorf("error %q should point at the %s escape hatch", err, EnvConfigYAML)
	}
}
