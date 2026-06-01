// Package config loads the NabuGate YAML config and builds the live provider
// adapters and the alias -> model routing table.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"nabugate/internal/provider"
)

// Config is the top-level configuration file structure.
type Config struct {
	Server    ServerConfig              `yaml:"server"`
	Providers map[string]ProviderConfig `yaml:"providers"`
	Models    map[string]ModelRoute     `yaml:"models"` // chat aliases
	Images    map[string]ModelRoute     `yaml:"images"` // image-generation aliases
	Audio     map[string]ModelRoute     `yaml:"audio"`  // text-to-speech aliases
}

// ServerConfig holds gateway listen options and the internal API keys that
// projects must present.
type ServerConfig struct {
	Port    int      `yaml:"port"`
	APIKeys []string `yaml:"api_keys"`
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

// Load reads and parses the config file at path.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
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
		if apiKey == "" {
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
