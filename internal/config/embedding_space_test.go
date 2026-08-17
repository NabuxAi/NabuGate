package config

import (
	"os"
	"testing"
)

// Aliases whose vectors are STORED by the consumer that asks for them.
//
// Each names a persisted index: NabuChat's Qdrant collection, NabuWrite's
// pgvector column, NabuDesk's Qdrant collections, NabuGen's content_memory.
// An alias in this list must resolve to exactly one provider/model pair.
var storedIndexAliases = []string{"chat-embed", "write-embed", "desk-embed", "gen-embed", "zooey-embed"}

// A fallback between two embedding models is not a fallback, it is two indexes.
//
// The rule that matters is the SPACE, not the width, and that distinction is
// what the comments alone failed to protect. write-embed's own comment named
// the hazard precisely — "split the corpus across two incompatible embedding
// spaces with no error at all" — and a later change added a gemini rung beside
// the OpenAI one anyway, on the reasoning that both are 1536 wide. Two 1536-dim
// models are two different geometries: measured through this gateway on
// identical text, correct pairs scored 0.189-0.323 under one and 0.663-0.691
// under the other. Cosine between a vector from one and a vector from the other
// is a number with no relationship to similarity.
//
// Nothing fails when that happens. The insert succeeds, the search returns
// rows, every score looks plausible, and recall decays with nothing in the
// logs. So the invariant is asserted here rather than left to a comment that
// has already been read and overridden once.
func TestStoredIndexAliasesHaveExactlyOneRung(t *testing.T) {
	cfg := loadDefaultConfig(t)

	for _, alias := range storedIndexAliases {
		route, ok := cfg.Embeddings[alias]
		if !ok {
			t.Errorf("default config should define embedding alias %q", alias)
			continue
		}
		// The invariant is one SPACE, not one rung. A fallback rung that names
		// the same model as the primary is the same weights reached over a
		// different transport — one geometry. A fallback that names a different
		// model is a second index, whatever its width. (2026-08: desk-embed
		// gained an openrouter rung carrying the identical OpenAI model after
		// parspack's wallet hit 402; that is the shape this permits.)
		for i, rung := range route.Fallback {
			if rung.Model != route.Primary.Model {
				t.Errorf("%s fallback rung %d names model %q, primary names %q; an alias backing a stored index "+
					"must stay in one embedding space, so every rung must carry the identical model — "+
					"a different model writes a second geometry into the same collection with no error raised",
					alias, i, rung.Model, route.Primary.Model)
			}
		}
		if route.Primary.Provider == "" || route.Primary.Model == "" {
			t.Errorf("%s: primary must name a provider and a model, got %+v", alias, route.Primary)
		}
	}
}

// Every embedding alias must be classified, because the two kinds have
// opposite rules.
//
// A stored-index alias must have exactly one rung. A query-time alias may cross
// widths and spaces freely, because nothing it produces is kept — nabu-embed
// does exactly that on purpose.
//
// The failure this guards is a new alias added without anyone deciding which it
// is. That is how write-embed acquired a second rung: not from ignorance of the
// rule, but because the question was never put. An unclassified alias fails
// here until someone answers it.
func TestEveryEmbeddingAliasIsClassified(t *testing.T) {
	cfg := loadDefaultConfig(t)

	// Aliases whose vectors are consumed immediately and never stored.
	queryTimeAliases := map[string]bool{
		"nabu-embed":  true,
		"qwen-embed":  true,
		"local-embed": true,
	}

	stored := map[string]bool{}
	for _, a := range storedIndexAliases {
		stored[a] = true
	}

	for alias := range cfg.Embeddings {
		if stored[alias] == queryTimeAliases[alias] {
			t.Errorf("embedding alias %q is not classified: add it to storedIndexAliases "+
				"(vectors are kept — then it must have exactly one rung) or to "+
				"queryTimeAliases (vectors are used once and discarded)", alias)
		}
	}

	// And the query-time ones must not be quietly reclassified by being given a
	// single rung and then used for storage; that is a decision, not a tidy-up.
	for alias := range queryTimeAliases {
		if _, ok := cfg.Embeddings[alias]; !ok {
			t.Errorf("query-time alias %q is no longer defined; if it was removed, "+
				"remove it here too rather than leaving a stale expectation", alias)
		}
	}
}

func TestStoredIndexAliasesPinTheirWidthOrLeaveItToTheCaller(t *testing.T) {
	// Not a width assertion on the config — the width comes from the caller's
	// `dimensions` — but the models here must be ones that honour it. This
	// catches an alias being pointed at a provider the gateway cannot pass
	// `dimensions` through to, which is how a 3072-wide vector reaches a
	// 1536-wide column.
	cfg := loadDefaultConfig(t)

	for _, alias := range storedIndexAliases {
		route, ok := cfg.Embeddings[alias]
		if !ok {
			continue
		}
		rungs := append([]Target{route.Primary}, route.Fallback...)
		for _, rung := range rungs {
			p, ok := cfg.Providers[rung.Provider]
			if !ok {
				t.Errorf("%s points at provider %q, which the config does not define",
					alias, rung.Provider)
				continue
			}
			switch p.Type {
			case "openai", "gemini":
				// Both forward the client's `dimensions`.
			default:
				t.Errorf("%s: provider %q is type %q; a stored index needs a provider that "+
					"forwards the caller's dimensions", alias, rung.Provider, p.Type)
			}
		}
	}
}

func loadDefaultConfig(t *testing.T) *Config {
	t.Helper()
	t.Setenv("NABU_API_KEY", "admin-from-env")

	raw, err := os.ReadFile("../../config.default.yaml")
	if err != nil {
		t.Fatalf("read config.default.yaml: %v", err)
	}
	cfg, err := Parse(string(raw))
	if err != nil {
		t.Fatalf("parse config.default.yaml: %v", err)
	}
	return cfg
}
