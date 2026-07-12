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
