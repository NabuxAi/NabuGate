// Package config loads the NabuGate YAML config and builds the live provider
// adapters and the alias -> model routing table.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/usage"
)

// EnvConfigYAML, when set to a non-empty value, supplies the whole config file
// inline instead of reading it from disk. This is the mount-free option for
// PaaS deploys (Coolify, Railway, …) where a bind mount of a not-yet-existing
// host file would otherwise be auto-created by Docker as an empty directory and
// crash the gateway on start.
const EnvConfigYAML = "NABU_CONFIG_YAML"

// Config is the top-level configuration file structure.
type Config struct {
	Server     ServerConfig              `yaml:"server"`
	Providers  map[string]ProviderConfig `yaml:"providers"`
	Models     map[string]ModelRoute     `yaml:"models"`     // chat aliases
	Images     map[string]ModelRoute     `yaml:"images"`     // image-generation aliases
	Audio      map[string]ModelRoute     `yaml:"audio"`      // text-to-speech aliases
	Embeddings map[string]ModelRoute     `yaml:"embeddings"` // text-embedding aliases
	Pricing    map[string]usage.Price    `yaml:"pricing"`    // USD per 1M tokens, keyed by "provider/model"
}

// ServerConfig holds gateway listen options and the internal API keys that
// projects must present. APIKeys is the simple full-access form; Keys is the
// rich per-project form with allow-lists and rate limits.
type ServerConfig struct {
	Port    int                `yaml:"port"`
	APIKeys []string           `yaml:"api_keys"`
	Keys    []policy.KeyConfig `yaml:"keys"`
}

// ProviderConfig describes one upstream provider.
type ProviderConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Type      string `yaml:"type"` // "openai" | "anthropic" | "gemini"
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// Target points at a concrete provider + upstream model name.
type Target struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// ModelRoute maps a public alias (e.g. "nabu-fast") to a primary target and an
// ordered list of fallbacks.
type ModelRoute struct {
	Primary  Target   `yaml:"primary"`
	Fallback []Target `yaml:"fallback"`
}

// Resolve loads the config from the NABU_CONFIG_YAML env var when it is set
// (the mount-free path for PaaS deploys), otherwise from the file at path.
// Inline config takes precedence so a stale or auto-created bind-mount file
// cannot shadow it.
func Resolve(path string) (*Config, error) {
	if inline := strings.TrimSpace(os.Getenv(EnvConfigYAML)); inline != "" {
		cfg, err := Parse(inline)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", EnvConfigYAML, err)
		}
		return cfg, nil
	}
	return Load(path)
}

// Load reads and parses the config file at path. Any ${VAR} references in the
// file are expanded from the environment first, so secrets (gateway API keys,
// etc.) can be injected at runtime instead of baked into the file.
func Load(path string) (*Config, error) {
	// A bind mount whose host source is missing makes Docker auto-create the
	// target as an empty *directory*; os.ReadFile then fails with the cryptic
	// "is a directory". Detect that case and explain it.
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return nil, fmt.Errorf("config path %q is a directory, not a file "+
			"(a Docker bind mount with a missing host file creates an empty "+
			"directory — mount a real config file, remove the mount, or supply "+
			"the config inline via the %s env var)", path, EnvConfigYAML)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file %q not found "+
				"(mount a config file there or supply it inline via the %s env var)", path, EnvConfigYAML)
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	return Parse(string(raw))
}

// Parse builds a Config from raw YAML content. Any ${VAR} references are
// expanded from the environment first, so secrets can be injected at runtime
// instead of baked into the file. It is shared by file and inline (env) loading.
func Parse(raw string) (*Config, error) {
	expanded := os.ExpandEnv(raw)
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	return &cfg, nil
}

// BuildAdapters instantiates an adapter for every enabled provider. Providers
// whose API key env var is unset are skipped with a warning so the gateway can
// still start with a subset of providers configured.
func (c *Config) BuildAdapters() (map[string]provider.Adapter, []string) {
	adapters := make(map[string]provider.Adapter)
	var warnings []string

	for name, p := range c.Providers {
		if !p.Enabled {
			continue
		}
		apiKey := os.Getenv(p.APIKeyEnv)
		if strings.TrimSpace(p.APIKeyEnv) == "" {
			// Keyless provider (e.g. a self-hosted Ollama endpoint): it declares
			// no api_key_env, so it is enabled purely by having a base_url. The
			// OpenAI-wire adapter still sends a placeholder bearer token, which
			// such local endpoints ignore. Only OpenAI-wire providers may be
			// keyless — Anthropic/Gemini always need a real key, so a missing
			// api_key_env there is a misconfiguration, not a local endpoint.
			if p.Type != "openai" {
				warnings = append(warnings, fmt.Sprintf("provider %q disabled: %q providers require an api_key_env", name, p.Type))
				continue
			}
			if strings.TrimSpace(p.BaseURL) == "" {
				warnings = append(warnings, fmt.Sprintf("provider %q disabled: keyless provider needs a base_url", name))
				continue
			}
			if apiKey == "" {
				apiKey = "-" // placeholder; keyless local endpoints ignore it
			}
		} else if apiKey == "" {
			warnings = append(warnings, fmt.Sprintf("provider %q disabled: env %s is empty", name, p.APIKeyEnv))
			continue
		}

		switch p.Type {
		case "openai":
			var extra map[string]string
			// OpenRouter recommends (but does not require) attribution headers.
			adapters[name] = provider.NewOpenAIAdapter(name, p.BaseURL, apiKey, extra)
		case "anthropic":
			adapters[name] = provider.NewAnthropicAdapter(name, p.BaseURL, apiKey)
		case "gemini":
			adapters[name] = provider.NewGeminiAdapter(name, p.BaseURL, apiKey)
		default:
			warnings = append(warnings, fmt.Sprintf("provider %q has unknown type %q", name, p.Type))
		}
	}

	return adapters, warnings
}
