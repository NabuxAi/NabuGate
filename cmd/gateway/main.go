// Command gateway starts the NabuGate AI/LLM gateway.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/router"
	"nabugate/internal/server"
	"nabugate/internal/usage"
)

func main() {
	configPath := flag.String("config", envOr("NABU_CONFIG", "config.yaml"),
		"path to the YAML config file (ignored when the NABU_CONFIG_YAML env var holds the config inline)")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Resolve(*configPath)
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	adapters, warnings := cfg.BuildAdapters()
	for _, w := range warnings {
		log.Warn(w)
	}
	if len(adapters) == 0 {
		log.Error("no providers available; set provider API keys and try again")
		os.Exit(1)
	}

	passthrough := cfg.Passthroughs(adapters)
	r := router.New(adapters, cfg.Models, cfg.Images, cfg.Audio, cfg.Embeddings, passthrough, log)
	enforcer := policy.New(cfg.Server.APIKeys, cfg.Server.Keys)
	tracker := usage.New(cfg.Pricing)
	agents, agentWarnings := cfg.BuildAgents()
	for _, w := range agentWarnings {
		log.Warn(w)
	}
	srv := server.New(r, enforcer, tracker, agents, log)

	if !enforcer.Enabled() {
		// A gateway that holds provider secrets and spends money must not come up
		// open by accident (e.g. NABU_API_KEY left unset). Fail closed unless the
		// operator explicitly opts into an unauthenticated gateway.
		if os.Getenv("NABU_ALLOW_NO_AUTH") == "" {
			log.Error("refusing to start with authentication disabled: no api keys configured " +
				"(set NABU_API_KEY, or server.api_keys in your config; " +
				"set NABU_ALLOW_NO_AUTH=1 to run an open gateway for local dev)")
			os.Exit(1)
		}
		log.Warn("authentication is DISABLED (NABU_ALLOW_NO_AUTH set): the gateway is open to anyone who can reach it")
	}

	providerNames := make([]string, 0, len(adapters))
	for name := range adapters {
		providerNames = append(providerNames, name)
	}
	passthroughNames := make([]string, 0, len(passthrough))
	for name := range passthrough {
		passthroughNames = append(passthroughNames, name)
	}
	log.Info("nabugate starting",
		"port", cfg.Server.Port,
		"providers", providerNames,
		"aliases", r.Aliases(),
		"passthrough", passthroughNames,
		"agents", agents.Names(),
	)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// Bound how long a client may take to send its request body (slow-loris);
		// intentionally no WriteTimeout, which would sever long SSE streams.
		ReadTimeout: 60 * time.Second,
	}
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
