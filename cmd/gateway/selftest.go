package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"nabugate/internal/config"
	"nabugate/internal/provider"
	"nabugate/internal/router"
)

// runSelfTest asks every advertised alias to do the smallest real piece of work
// it can, and reports which ones actually answer.
//
// `/healthz` says the process is up. That is not the question anyone has: a
// gateway answers /healthz perfectly while serving none of the aliases its
// consumers ask for, and the first sign is an opaque failure inside somebody
// else's product. Every alias verified by hand this week — a chat completion,
// an embedding, an image, a speech clip — had to be checked one request at a
// time, and none of that was repeatable by the next person.
//
// Chat and embedding aliases are exercised by default because they are cheap:
// one token and one character. Image and audio cost real money per call, so
// they are opt-in behind -selftest-all rather than billed to whoever runs a
// health check.
func runSelfTest(cfg *config.Config, all bool) int {
	adapters, warnings := cfg.BuildAdapters()
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning: "+w)
	}
	if len(adapters) == 0 {
		fmt.Fprintln(os.Stderr, "no providers available; set provider API keys and try again")
		return 1
	}

	r := router.New(adapters, cfg.Models, cfg.Images, cfg.Audio, cfg.Embeddings,
		cfg.Transcription, cfg.Passthroughs(adapters),
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.SetRegistry(cfg.Registry)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	type result struct {
		kind, alias, detail string
		ok                  bool
	}
	var results []result

	check := func(kind, alias string, fn func() (string, error)) {
		detail, err := fn()
		if err != nil {
			// One line, not a stack: this is read by someone deciding which
			// alias to fix, and the provider's own message is the useful part.
			msg := err.Error()
			if i := strings.Index(msg, "\n"); i > 0 {
				msg = msg[:i]
			}
			if len(msg) > 140 {
				msg = msg[:140] + "…"
			}
			results = append(results, result{kind, alias, msg, false})
			return
		}
		results = append(results, result{kind, alias, detail, true})
	}

	for _, alias := range sortedKeys(cfg.Models) {
		check("chat", alias, func() (string, error) {
			maxTokens := 1
			res, err := r.Chat(ctx, alias, provider.ChatRequest{
				Messages:  []provider.Message{{Role: "user", Content: "hi"}},
				MaxTokens: &maxTokens,
			})
			if err != nil {
				return "", err
			}
			return res.Provider + "/" + res.Model, nil
		})
	}

	for _, alias := range sortedKeys(cfg.Embeddings) {
		check("embedding", alias, func() (string, error) {
			res, err := r.Embed(ctx, alias, provider.EmbeddingRequest{Input: []string{"x"}})
			if err != nil {
				return "", err
			}
			natural := 0
			if len(res.Embeddings) > 0 {
				natural = len(res.Embeddings[0])
			}

			// Then ask for a specific width, because that is the property every
			// consumer storing these vectors depends on. gemini-embedding-001
			// answers 3072 by default; a caller with a vector(1536) column sends
			// `dimensions` and gets 1536 — unless the alias silently drops it,
			// in which case the insert fails or the column is corrupted, far
			// from here and with nothing pointing back.
			const want = 1536
			w := want
			pinned, err := r.Embed(ctx, alias, provider.EmbeddingRequest{
				Input: []string{"x"}, Dimensions: &w,
			})
			if err != nil {
				return "", fmt.Errorf("natural width %d, but requesting %d failed: %w", natural, want, err)
			}
			got := 0
			if len(pinned.Embeddings) > 0 {
				got = len(pinned.Embeddings[0])
			}
			if got != want {
				return "", fmt.Errorf(
					"asked for %d dimensions and got %d — a consumer with a fixed-width column would "+
						"write a wrong-width vector or fail its insert", want, got)
			}
			return fmt.Sprintf("%s/%s %d dims natural, honours dimensions=%d",
				res.Provider, res.Model, natural, want), nil
		})
	}

	if all {
		for _, alias := range sortedKeys(cfg.Images) {
			check("image", alias, func() (string, error) {
				res, err := r.Image(ctx, alias, provider.ImageRequest{Prompt: "a plain grey square", N: 1})
				if err != nil {
					return "", err
				}
				n := 0
				if len(res.Images) > 0 {
					n = len(res.Images[0])
				}
				return fmt.Sprintf("%s/%s %d base64 chars", res.Provider, res.Model, n), nil
			})
		}
		for _, alias := range sortedKeys(cfg.Audio) {
			check("audio", alias, func() (string, error) {
				res, err := r.Speech(ctx, alias, provider.SpeechRequest{Input: "test", Voice: "alloy"})
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("%s/%s %d bytes %s", res.Provider, res.Model, len(res.Audio), res.ContentType), nil
			})
		}
	}

	failed := 0
	for _, res := range results {
		mark := "ok  "
		if !res.ok {
			mark = "FAIL"
			failed++
		}
		fmt.Printf("%s  %-10s %-20s %s\n", mark, res.kind, res.alias, res.detail)
	}

	fmt.Printf("\n%d/%d aliases answered", len(results)-failed, len(results))
	if !all {
		fmt.Print("  (image and audio skipped; -selftest-all includes them, they cost money per call)")
	}
	fmt.Println()

	// Non-zero on any failure, so this is usable as a deployment gate rather
	// than something to read and forget.
	if failed > 0 {
		return 1
	}
	return 0
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
