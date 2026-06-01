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
	configPath := flag.String("config", envOr("NABU_CONFIG", "config.yaml"), "path to the YAML config file")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*configPath)
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

	r := router.New(adapters, cfg.Models, cfg.Images, cfg.Audio, cfg.Embeddings, log)
	enforcer := policy.New(cfg.Server.APIKeys, cfg.Server.Keys)
	tracker := usage.New(cfg.Pricing)
	srv := server.New(r, enforcer, tracker, log)

	if !enforcer.Enabled() {
		log.Warn("no api keys configured: authentication is DISABLED (dev mode)")
	}

	providerNames := make([]string, 0, len(adapters))
	for name := range adapters {
		providerNames = append(providerNames, name)
	}
	log.Info("nabugate starting",
		"port", cfg.Server.Port,
		"providers", providerNames,
		"aliases", r.Aliases(),
	)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
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
