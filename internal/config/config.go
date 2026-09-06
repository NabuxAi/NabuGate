// Package config loads the NabuGate YAML config and builds the live provider
// adapters and the alias -> model routing table.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"nabugate/internal/agent"
	"nabugate/internal/flow"
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
	Server    ServerConfig              `yaml:"server"`
	Providers map[string]ProviderConfig `yaml:"providers"`
	Models    map[string]ModelRoute     `yaml:"models"` // chat aliases
	Images    map[string]ModelRoute     `yaml:"images"` // image-generation aliases
	Audio     map[string]ModelRoute     `yaml:"audio"`  // text-to-speech aliases
	// Speech-to-text. Separate from Audio because they are opposite directions
	// with different upstream models, and one alias cannot serve both.
	Transcription map[string]ModelRoute  `yaml:"transcription"`
	Embeddings    map[string]ModelRoute  `yaml:"embeddings"` // text-embedding aliases
	Pricing       map[string]usage.Price `yaml:"pricing"`    // USD per 1M tokens, keyed by "provider/model"

	// Registry maps a logical model name to the providers that can serve it.
	// A target naming a model with no provider expands through this, so one
	// provider going down switches to the next without the caller noticing.
	Registry map[string]ModelEntry `yaml:"model_registry"`

	// Agents are named sub-agents (system prompt + defaults over a chat alias),
	// addressable as a "model". They may be declared inline here or, so they can
	// be authored and dropped in from outside the main config, as one-file-each
	// YAML in AgentsDir.
	Agents    map[string]AgentConfig `yaml:"agents"`
	AgentsDir string                 `yaml:"agents_dir"`

	// Flows are named chains of agents, each step handed what the one before
	// it produced, and are addressable as a "model" the same way agents are.
	// Declared inline here or one-file-each in FlowsDir, for the same reason.
	Flows    map[string]FlowConfig `yaml:"flows"`
	FlowsDir string                `yaml:"flows_dir"`

	// MCP is the Model Context Protocol endpoint (see the MCP type).
	MCP MCP `yaml:"mcp"`

	// Plans are the subscriptions a user can buy to spend the gateway's own
	// vendor keys. No plans declared means nobody can buy one, and the only way
	// to reach a gated provider stays an admin's approval — which is exactly
	// how every deployment behaves before this file gains a `plans:` block.
	Plans []Plan `yaml:"plans"`
}

// Plan is one subscription offer.
//
// It sells two things at once and they are priced separately on purpose: the
// fee unlocks the gateway's credentials, and Markup sets what each call served
// on them costs against the user's balance. A caller using their own key is
// touched by neither.
type Plan struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	// Description is shown on the plan card; one sentence, in the console's
	// language, not English-only.
	Description string `yaml:"description"`

	PriceUSD float64 `yaml:"price_usd"`
	// Days is one term. 30 is a month here; nothing computes calendar months,
	// because a term that ends on a different day of the month each time is a
	// support ticket waiting to happen.
	Days int `yaml:"days"`

	// Providers are globs (`*`, `gem*`, `openai`) naming whose gateway key this
	// plan unlocks. Empty unlocks nothing, which makes an incomplete plan
	// useless rather than dangerously generous.
	Providers []string `yaml:"providers"`

	// Markup multiplies the metered cost of a call served on the gateway's key.
	// Unset or below 1 means at cost.
	Markup float64 `yaml:"markup"`
	// MinChargeUSD floors one such call. Without it a model with no `pricing:`
	// entry meters at zero and any markup of zero is still zero — the gateway
	// would hand out its own credentials free of charge and the console would
	// show a busy month that earned nothing.
	MinChargeUSD float64 `yaml:"min_charge_usd"`

	// IncludesCreditUSD is credit granted on purchase, for a plan priced as a
	// bundle rather than as pure access.
	IncludesCreditUSD float64 `yaml:"includes_credit_usd"`

	// Popular marks the one the console highlights.
	Popular bool `yaml:"popular"`
}

// Term returns the plan's term in days, defaulting to a month.
func (p Plan) Term() int {
	if p.Days <= 0 {
		return 30
	}
	return p.Days
}

// Rate returns the multiplier to apply to a gateway-served call, never below 1:
// a plan that discounted the gateway's own cost would bill less than the call
// spent, and no configuration mistake should be able to do that.
func (p Plan) Rate() float64 {
	if p.Markup < 1 {
		return 1
	}
	return p.Markup
}

// Plan looks up one offer by id.
func (c *Config) Plan(id string) (Plan, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, p := range c.Plans {
		if strings.ToLower(strings.TrimSpace(p.ID)) == id {
			return p, true
		}
	}
	return Plan{}, false
}

// MCP is the Model Context Protocol endpoint: a read-only view of this
// service for AI clients, on its own path behind its own token.
//
// TokenEnv names the variable the token arrives in rather than holding it,
// exactly like every other credential in this file, so the config stays
// committable. An unset variable disables the endpoint — the route is not
// mounted at all. There is no unauthenticated mode.
type MCP struct {
	Enabled  bool   `yaml:"enabled"`
	Path     string `yaml:"path"`
	TokenEnv string `yaml:"token_env"`
}

// FlowConfig is one flow as written in YAML. The name comes from the map key
// (inline) or the file name (flows_dir) unless Name overrides it.
type FlowConfig struct {
	Name        string           `yaml:"name"`
	Description string           `yaml:"description"`
	Steps       []FlowStepConfig `yaml:"steps"`
}

// FlowStepConfig is one link. `input` is a template; left out, the step simply
// receives the previous step's output, which is what a chain usually means.
type FlowStepConfig struct {
	Agent    string `yaml:"agent"`
	Label    string `yaml:"label"`
	Input    string `yaml:"input"`
	Optional bool   `yaml:"optional"`
}

// AgentConfig is one sub-agent as written in YAML. The agent name comes from the
// map key (inline) or the file name (agents_dir) unless Name overrides it.
type AgentConfig struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Model       string   `yaml:"model"` // underlying chat alias or "<provider>/<model>"
	System      string   `yaml:"system"`
	Temperature *float64 `yaml:"temperature"`
	TopP        *float64 `yaml:"top_p"`
	MaxTokens   *int     `yaml:"max_tokens"`

	// Tools are server-side functions the agent's model may call (see
	// agents/README.md). MaxToolSteps bounds the tool-call loop.
	Tools        []ToolConfig `yaml:"tools"`
	MaxToolSteps int          `yaml:"max_tool_steps"`
}

// ToolConfig is one agent tool as written in YAML. Parameters is kept as a
// free-form map because it is a JSON schema — the gateway does not interpret
// it, it forwards it to the model verbatim as the function signature.
type ToolConfig struct {
	Name         string            `yaml:"name"`
	Type         string            `yaml:"type"` // "http" (the only executor so far)
	Description  string            `yaml:"description"`
	Method       string            `yaml:"method"`
	URL          string            `yaml:"url"`
	Headers      map[string]string `yaml:"headers"`
	PathParams   []string          `yaml:"path_params"`
	BodyTemplate map[string]any    `yaml:"body_template"`
	Parameters   map[string]any    `yaml:"parameters"`

	TimeoutMS        int `yaml:"timeout_ms"`
	MaxResponseBytes int `yaml:"max_response_bytes"`
}

// ServerConfig holds gateway listen options and the internal API keys that
// projects must present. APIKeys is the simple full-access form; Keys is the
// rich per-project form with allow-lists and rate limits.
type ServerConfig struct {
	Port    int                `yaml:"port"`
	APIKeys []string           `yaml:"api_keys"`
	Keys    []policy.KeyConfig `yaml:"keys"`

	// CORSOrigins are the browser origins allowed to call the gateway. Empty
	// sends no CORS headers at all, which is what the gateway did before this
	// existed — so an upgrade opens nothing. Patterns are the same shape as a
	// key's allowed_origins: a bare host, or "*.example.com" for a subdomain
	// tree. It is deliberately separate from that per-key check, which still
	// runs: this says whether the browser may make the call, that says whether
	// the key may be used from there.
	CORSOrigins []string `yaml:"cors_origins"`
}

// ProviderConfig describes one upstream provider.
type ProviderConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Type      string `yaml:"type"` // "openai" | "anthropic" | "gemini" | "pexels"
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`

	// AuthScheme overrides the Authorization header scheme for an OpenAI-wire
	// provider. It defaults to "Bearer" (empty or "bearer" keeps that default);
	// set it to e.g. "apikey" for gateways that expect "Authorization: apikey
	// <key>" instead of a bearer token (ArvanCloud AIaaS does). Ignored by the
	// non-OpenAI adapters.
	AuthScheme string `yaml:"auth_scheme"`

	// TranscribeFormat overrides the response_format sent to
	// /audio/transcriptions on an OpenAI-wire provider. Empty keeps
	// verbose_json, which is the only shape carrying language, duration and
	// segments. Set it to "json" for a vendor that speaks the wire format
	// without implementing verbose_json — Mistral's Voxtral does not.
	TranscribeFormat string `yaml:"transcribe_format"`

	// Access decides whether a console user may spend *the gateway's* key on
	// this provider without asking. "auto" (the default, and what every
	// provider shipped here uses) grants it on request with no human in the
	// loop; "request" holds the ask for an admin. It never affects a caller
	// using their own key, and never affects the deployment's own baked keys —
	// only tokens minted by a console user. Turning a provider to "request"
	// therefore cannot break an existing integration.
	Access string `yaml:"access"`

	// Passthrough turns the provider into a first-class multi-model provider:
	// callers may address any of its models directly as "<provider>/<model>"
	// (e.g. "parspack/openai/gpt-5.5") with no hand-written alias, and — for
	// OpenAI-wire providers — the provider's whole catalogue is discovered live
	// from its /v1/models endpoint and surfaced on the gateway's own /v1/models.
	Passthrough bool `yaml:"passthrough"`
	// Models is an optional static catalogue for a passthrough provider. It is
	// always listed on /v1/models (in addition to anything discovered live), so
	// providers without a usable /v1/models endpoint can still advertise models.
	Models []string `yaml:"models"`
}

// Target points at an upstream model.
//
// With Provider set it is a concrete coordinate. With Provider empty, Model
// names an entry in the model registry and the router expands it into one
// target per serving provider.
type Target struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`

	// ParamStyle names the parameter dialect this model speaks. Empty is the
	// classic OpenAI chat contract. "reasoning" is the gpt-5.x / o-series
	// contract: max_tokens was replaced by max_completion_tokens, and
	// temperature/top_p accept only their default of 1.
	//
	// Sending the classic parameters to a reasoning model is rejected upstream,
	// and some proxies surface that rejection as an HTML error page rather than
	// a JSON error — which reads as an empty completion and is very hard to
	// trace back. Absorbing that difference is the gateway's job; a caller
	// should not have to know which dialect a model behind an alias speaks.
	ParamStyle string `yaml:"param_style"`
}

// ModelEntry describes one logical model and every provider that can serve it,
// in preference order.
//
// A model is an identity, not a provider coordinate: "gpt-5.5" is the same
// model whether Parspack, AvalAI or GapGPT is serving it. Without this, every
// alias has to repeat the provider list by hand, and when one provider breaks
// the fix has to be applied in each place it was copied to.
//
// A target may then name a model without a provider, and the router expands it
// into one attempt per serving provider — so a provider failing mid-chain is
// absorbed silently and the caller never learns it happened.
type ModelEntry struct {
	// ParamStyle is a property of the model, not of who serves it, so it is
	// declared once here. A Serving entry may still override it for a provider
	// that wraps the model unusually.
	ParamStyle string `yaml:"param_style"`

	// Serves lists the providers that can serve this model, best first.
	Serves []Serving `yaml:"serves"`
}

// Serving is one provider's coordinate for a logical model.
type Serving struct {
	Provider string `yaml:"provider"`
	// Model is the upstream name, which differs per provider: the same model is
	// "openai/gpt-5.5" on Parspack and "gpt-5.5" on AvalAI.
	Model      string `yaml:"model"`
	ParamStyle string `yaml:"param_style"`
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

		a, warning := newAdapter(name, p, apiKey)
		if a == nil {
			warnings = append(warnings, warning)
			continue
		}
		adapters[name] = a
	}

	return adapters, warnings
}

// newAdapter builds one adapter from a provider's config and an explicit key.
// It is separate from BuildAdapters because the key is a parameter rather than
// an environment lookup: the same provider is also built per-request against a
// key the caller supplied, and the two paths must agree on base URL, auth
// scheme and adapter type or a caller's own key would behave differently from
// the gateway's. A nil adapter comes with a warning explaining why.
func newAdapter(name string, p ProviderConfig, apiKey string) (provider.Adapter, string) {
	switch p.Type {
	case "openai":
		// Every OpenAI-wire provider needs a base_url (keyless ones are checked
		// by the caller; keyed ones — e.g. an ArvanCloud endpoint whose
		// ${ARVAN_AIAAS_ENDPOINT} was left unset — are checked here so they are
		// skipped with a clear warning instead of building an adapter that would
		// fail every request against an empty URL).
		if strings.TrimSpace(p.BaseURL) == "" {
			return nil, fmt.Sprintf("provider %q disabled: openai providers need a base_url", name)
		}
		// authHeaderOverride is non-nil only when the provider asks for a
		// non-Bearer Authorization scheme; the adapter applies these extra
		// headers over its Bearer default across every endpoint it calls.
		oa := provider.NewOpenAIAdapter(name, p.BaseURL, apiKey, authHeaderOverride(p.AuthScheme, apiKey))
		oa.SetTranscribeFormat(strings.TrimSpace(p.TranscribeFormat))
		return oa, ""
	case "anthropic":
		return provider.NewAnthropicAdapter(name, p.BaseURL, apiKey), ""
	case "gemini":
		return provider.NewGeminiAdapter(name, p.BaseURL, apiKey), ""
	case "pexels":
		return provider.NewPexelsAdapter(name, p.BaseURL, apiKey), ""
	case "elevenlabs":
		// Speech both ways: text-to-speech, and Scribe for transcription. It
		// authenticates with xi-api-key and puts the voice in the URL, so it
		// cannot ride the openai adapter.
		return provider.NewElevenLabsAdapter(name, p.BaseURL, apiKey), ""
	case "imagegen":
		// mrc_imagegen: a template renderer, not a diffusion model. See the
		// adapter for how a prompt maps onto its fields.
		return provider.NewImagegenAdapter(name, p.BaseURL, apiKey), ""
	case "deepgram":
		// Pre-recorded transcription. The audio is the request body and the
		// options are query parameters, and the auth scheme is "Token" — none
		// of which the openai adapter can express.
		return provider.NewDeepgramAdapter(name, p.BaseURL, apiKey), ""
	case "assemblyai":
		// Upload, submit, poll — three calls to one transcript, all inside one
		// synchronous Transcribe.
		return provider.NewAssemblyAIAdapter(name, p.BaseURL, apiKey), ""
	case "speechmatics":
		// Batch transcription only, and asynchronous: the adapter submits a job
		// and polls it to completion inside one call. Neither shape fits the
		// openai adapter.
		return provider.NewSpeechmaticsAdapter(name, p.BaseURL, apiKey), ""
	case "gamma":
		// gamma.app: decks, documents and social posts. Asynchronous, and a chat
		// adapter rather than an image one because what comes back is a hosted
		// URL. See the adapter.
		return provider.NewGammaAdapter(name, p.BaseURL, apiKey), ""
	default:
		return nil, fmt.Sprintf("provider %q has unknown type %q", name, p.Type)
	}
}

// CallerAdapter builds an adapter for one provider against a key the caller
// sent with a request, so the gateway can spend the caller's credential instead
// of its own. It refuses a provider this deployment does not define or has
// switched off: a key is not permission to reach an arbitrary URL through here.
//
// The gateway's own key is never consulted, and nothing is cached — the adapter
// lives as long as the request that carried the key.
func (c *Config) CallerAdapter(name, apiKey string) (provider.Adapter, bool) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, false
	}
	p, ok := c.Providers[name]
	if !ok || !p.Enabled {
		return nil, false
	}
	// A keyless provider is a local endpoint that ignores credentials; handing
	// it someone's key would silently do nothing.
	if strings.TrimSpace(p.APIKeyEnv) == "" {
		return nil, false
	}
	a, _ := newAdapter(name, p, apiKey)
	return a, a != nil
}

// authHeaderOverride returns the extra headers that make the OpenAI adapter use
// a non-Bearer Authorization scheme, or nil to keep its "Bearer <key>" default.
// Some OpenAI-wire gateways expect a different scheme — ArvanCloud AIaaS wants
// "Authorization: apikey <key>" — which the adapter honours because it applies
// caller-supplied headers over the default Authorization on every request.
func authHeaderOverride(scheme, apiKey string) map[string]string {
	scheme = strings.TrimSpace(scheme)
	if scheme == "" || strings.EqualFold(scheme, "bearer") {
		return nil
	}
	return map[string]string{"Authorization": scheme + " " + apiKey}
}

// BuildAgents assembles the sub-agent registry from the inline `agents:` map and,
// when `agents_dir` is set, from every *.yaml/*.yml file in that directory (one
// agent per file). Invalid or duplicate agents are skipped with a warning rather
// than aborting startup, mirroring how a provider with a missing key is skipped:
// one bad definition must not take the whole gateway down. Inline agents are
// registered first, so an inline name wins over a same-named file.
func (c *Config) BuildAgents() (*agent.Registry, []string) {
	reg := agent.NewRegistry()
	var warnings []string

	// Inline agents. Iterate in sorted key order for deterministic warnings.
	keys := make([]string, 0, len(c.Agents))
	for key := range c.Agents {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		ac := c.Agents[key]
		name := strings.TrimSpace(ac.Name)
		if name == "" {
			name = key
		}
		if err := reg.Add(ac.toAgent(name)); err != nil {
			warnings = append(warnings, fmt.Sprintf("skip inline agent %q: %v", key, err))
		}
	}

	// External agent files, so specialists can be defined from outside the main
	// config and version-controlled alongside the project that uses them.
	if dir := strings.TrimSpace(c.AgentsDir); dir != "" {
		files, err := agentFiles(dir)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("agents_dir %q: %v", dir, err))
		}
		for _, path := range files {
			ac, err := loadAgentFile(path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("skip agent file %q: %v", path, err))
				continue
			}
			name := strings.TrimSpace(ac.Name)
			if name == "" {
				name = agentNameFromPath(path)
			}
			if err := reg.Add(ac.toAgent(name)); err != nil {
				warnings = append(warnings, fmt.Sprintf("skip agent file %q: %v", path, err))
			}
		}
	}

	return reg, warnings
}

// BuildFlows assembles the flow registry, the same way BuildAgents assembles
// agents and for the same reasons: inline first so an inline name wins over a
// same-named file, and one bad definition warns rather than aborting startup.
func (c *Config) BuildFlows() (*flow.Registry, []string) {
	reg := flow.NewRegistry()
	var warnings []string

	keys := make([]string, 0, len(c.Flows))
	for key := range c.Flows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fc := c.Flows[key]
		name := strings.TrimSpace(fc.Name)
		if name == "" {
			name = key
		}
		if err := reg.Add(fc.toFlow(name)); err != nil {
			warnings = append(warnings, fmt.Sprintf("skip inline flow %q: %v", key, err))
		}
	}

	if dir := strings.TrimSpace(c.FlowsDir); dir != "" {
		files, err := agentFiles(dir)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("flows_dir %q: %v", dir, err))
		}
		for _, path := range files {
			fc, err := loadFlowFile(path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("skip flow file %q: %v", path, err))
				continue
			}
			name := strings.TrimSpace(fc.Name)
			if name == "" {
				name = agentNameFromPath(path)
			}
			if err := reg.Add(fc.toFlow(name)); err != nil {
				warnings = append(warnings, fmt.Sprintf("skip flow file %q: %v", path, err))
			}
		}
	}

	return reg, warnings
}

func (fc FlowConfig) toFlow(name string) flow.Flow {
	steps := make([]flow.Step, 0, len(fc.Steps))
	for _, sc := range fc.Steps {
		steps = append(steps, flow.Step{
			Agent:    strings.TrimSpace(sc.Agent),
			Label:    sc.Label,
			Input:    sc.Input,
			Optional: sc.Optional,
		})
	}

	return flow.Flow{Name: name, Description: fc.Description, Steps: steps}
}

func loadFlowFile(path string) (FlowConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return FlowConfig{}, err
	}
	var fc FlowConfig
	if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(raw))), &fc); err != nil {
		return FlowConfig{}, err
	}
	return fc, nil
}

// toAgent converts a YAML AgentConfig into a runtime agent.Agent with the given
// resolved name. A tool whose parameters schema cannot be re-encoded as JSON
// (a YAML quirk such as a non-string map key) is left with empty Parameters,
// which Tool.Validate then reports as a missing schema at registration — the
// same "skip and warn" path as every other bad agent field.
func (ac AgentConfig) toAgent(name string) agent.Agent {
	return agent.Agent{
		Name:         name,
		Description:  ac.Description,
		Model:        ac.Model,
		System:       ac.System,
		Temperature:  ac.Temperature,
		TopP:         ac.TopP,
		MaxTokens:    ac.MaxTokens,
		Tools:        toolsToAgent(ac.Tools),
		MaxToolSteps: ac.MaxToolSteps,
	}
}

// toolsToAgent converts YAML tool declarations into runtime tools.
func toolsToAgent(cfgs []ToolConfig) []agent.Tool {
	if len(cfgs) == 0 {
		return nil
	}
	out := make([]agent.Tool, 0, len(cfgs))
	for _, tc := range cfgs {
		var params json.RawMessage
		if tc.Parameters != nil {
			if b, err := json.Marshal(tc.Parameters); err == nil {
				params = b
			}
		}
		out = append(out, agent.Tool{
			Name:             tc.Name,
			Type:             tc.Type,
			Description:      tc.Description,
			Method:           tc.Method,
			URL:              tc.URL,
			Headers:          tc.Headers,
			PathParams:       tc.PathParams,
			BodyTemplate:     tc.BodyTemplate,
			Parameters:       params,
			TimeoutMS:        tc.TimeoutMS,
			MaxResponseBytes: tc.MaxResponseBytes,
		})
	}
	return out
}

// agentFiles lists the *.yaml/*.yml files in dir, sorted for deterministic load
// order (which also fixes which of two same-named files wins).
func agentFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if ext := strings.ToLower(filepath.Ext(e.Name())); ext == ".yaml" || ext == ".yml" {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

// loadAgentFile reads and parses one agent definition file, expanding ${VAR}
// references from the environment just like the main config.
func loadAgentFile(path string) (AgentConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return AgentConfig{}, err
	}
	var ac AgentConfig
	if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(raw))), &ac); err != nil {
		return AgentConfig{}, err
	}
	return ac, nil
}

// agentNameFromPath derives an agent name from a file path (basename without
// extension), used when the file does not set `name:`.
func agentNameFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// Passthroughs returns the passthrough-enabled providers (name -> static model
// catalogue) that actually have a live adapter. Providers marked passthrough
// but skipped for a missing key are excluded, so the router never advertises or
// routes to a provider that could not be built. The static list may be empty;
// live discovery (for OpenAI-wire providers) supplements it at request time.
func (c *Config) Passthroughs(adapters map[string]provider.Adapter) map[string][]string {
	out := make(map[string][]string)
	for name, p := range c.Providers {
		if !p.Enabled || !p.Passthrough {
			continue
		}
		if _, ok := adapters[name]; !ok {
			continue
		}
		out[name] = p.Models
	}
	return out
}

// ProviderMeta is what the console needs to know about a provider that the
// router does not: whether this deployment defines it at all, and whether
// spending the gateway's key on it needs approval. The router only ever sees
// providers that came up.
type ProviderMeta struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Enabled     bool   `json:"enabled"`
	Passthrough bool   `json:"passthrough"`
	// KeyEnv is named, never read: it is what an operator has to set, and
	// showing it turns "why is this provider down" into a one-line answer.
	KeyEnv string `json:"key_env,omitempty"`
	// Access is "auto" or "request"; empty means auto.
	Access string `json:"access"`
	// BYOK says a caller's own key for this provider can actually be used. It
	// is false for a provider with no api_key_env — a local endpoint that
	// ignores credentials — and equally for one whose adapter cannot be built,
	// which is the case for any OpenAI-wire provider whose base_url comes from
	// an unset environment variable. Both would otherwise offer a form on the
	// console that saves a key every request then silently refuses.
	BYOK bool `json:"byok"`
}

// ProviderMetas describes every provider this deployment defines, live or not.
func (c *Config) ProviderMetas() map[string]ProviderMeta {
	out := make(map[string]ProviderMeta, len(c.Providers))
	for name, p := range c.Providers {
		access := strings.ToLower(strings.TrimSpace(p.Access))
		if access == "" {
			access = "auto"
		}
		// Ask the factory rather than guessing: it is the same code the
		// per-request path runs, so the console cannot advertise a credential
		// route that does not exist.
		byok := false
		if strings.TrimSpace(p.APIKeyEnv) != "" {
			a, _ := newAdapter(name, p, "probe")
			byok = a != nil
		}
		out[name] = ProviderMeta{
			Name:        name,
			Type:        p.Type,
			Enabled:     p.Enabled,
			Passthrough: p.Passthrough,
			KeyEnv:      p.APIKeyEnv,
			Access:      access,
			BYOK:        byok,
		}
	}
	return out
}
