// Command gateway starts the NabuGate AI/LLM gateway.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"nabugate/internal/adminstore"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"nabugate/internal/config"
	"nabugate/internal/photos"
	"nabugate/internal/policy"
	"nabugate/internal/router"
	"nabugate/internal/server"
	"nabugate/internal/usage"
	"nabugate/web"
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
	r.SetRegistry(cfg.Registry)
	enforcer := policy.New(cfg.Server.APIKeys, cfg.Server.Keys)
	tracker := usage.New(cfg.Pricing)
	agents, agentWarnings := cfg.BuildAgents()
	for _, w := range agentWarnings {
		log.Warn(w)
	}
	srv := server.New(r, enforcer, tracker, agents, log)

	// Console state: accounts, console-minted project tokens, and usage that
	// survives a restart. Mount a volume at NABU_STATE_DIR to keep it — without
	// one the file lives in the container and every redeploy loses the admin
	// account and the tokens minted from the console.
	stateDir := os.Getenv("NABU_STATE_DIR")
	if stateDir == "" {
		stateDir = "/data"
	}
	adminState, err := adminstore.Open(filepath.Join(stateDir, "console.json"))
	if err != nil {
		// Not fatal: the gateway's own routing does not depend on it, and
		// refusing to serve traffic because a console cannot start would be the
		// wrong trade.
		log.Warn("console state unavailable; admin console disabled", "error", err)
	} else {
		srv.SetAdminStore(adminState)
		if adminState.NeedsSetup() {
			log.Info("admin console has no account yet; create one at /admin/")
		}
		// Usage is accumulated in memory and flushed here, because a disk write
		// per request would cost more than a cheap completion does.
		go func() {
			t := time.NewTicker(30 * time.Second)
			defer t.Stop()
			for range t.C {
				if err := adminState.Persist(); err != nil {
					log.Warn("persist console usage", "error", err)
				}
			}
		}()
	}

	// Stock-photo proxy (Pexels): enabled purely by PEXELS_API_KEY, so photos
	// ride the same gateway key/policy as every other capability.
	if photoClient := photos.New(os.Getenv("PEXELS_API_KEY"), os.Getenv("PEXELS_BASE_URL")); photoClient != nil {
		srv.WithPhotos(photoClient)
		log.Info("photo proxy enabled (pexels)")
	} else {
		log.Info("photo proxy disabled (PEXELS_API_KEY not set)")
	}

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
	if _, ok := web.Assets(); ok {
		log.Info("admin console available", "path", "/admin/")
	}

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
