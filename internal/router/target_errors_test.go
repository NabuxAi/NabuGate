package router

import (
	"errors"
	"strings"
	"testing"
)

// Why every rung failed, not just the last one.
//
// Chasing a broken alias produced: `all targets failed for embedding alias
// "nabu-embed": provider "cloudflare" not available`. Cloudflare was the LAST
// rung and the least interesting. The primary had no API key and the middle
// rung named a model Google had retired — neither appeared, because the loop
// kept only the most recent error.
func TestTargetErrorsReportsEveryRung(t *testing.T) {
	var f targetErrors
	f.add("openai", "text-embedding-3-small", errors.New("provider not available (is its API key set?)"))
	f.add("gemini", "text-embedding-004", errors.New("upstream 404"))
	f.add("cloudflare", "@cf/baai/bge-large-en-v1.5", errors.New("provider not available"))

	msg := f.err("embedding", "nabu-embed").Error()

	for _, want := range []string{
		"nabu-embed",
		"openai/text-embedding-3-small",
		"API key",
		"gemini/text-embedding-004",
		"404",
		"cloudflare/@cf/baai/bge-large-en-v1.5",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should mention %q; got: %s", want, msg)
		}
	}
}

func TestTargetErrorsNamesTheAliasAndCapability(t *testing.T) {
	var f targetErrors
	f.add("openai", "m", errors.New("boom"))

	msg := f.err("embedding", "some-alias").Error()

	if !strings.Contains(msg, `embedding alias "some-alias"`) {
		t.Errorf("the alias and capability should both appear; got: %s", msg)
	}
}

func TestTargetErrorsWithNoTargetsSaysSo(t *testing.T) {
	// A route with an empty chain is a configuration mistake, and "no targets
	// configured" points at the config rather than at an upstream.
	var f targetErrors

	msg := f.err("embedding", "empty-alias").Error()

	if !strings.Contains(msg, "no targets configured") {
		t.Errorf("an empty chain should say so; got: %s", msg)
	}
}

func TestTargetErrorsKeepsOrder(t *testing.T) {
	// Primary first: the reader wants to know why the intended target failed
	// before reading about the substitutes.
	var f targetErrors
	f.add("primary-provider", "a", errors.New("first"))
	f.add("second-provider", "b", errors.New("second"))

	msg := f.err("chat", "x").Error()

	if strings.Index(msg, "primary-provider") > strings.Index(msg, "second-provider") {
		t.Errorf("the primary's failure should come first; got: %s", msg)
	}
}
