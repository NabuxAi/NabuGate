package web

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var (
	blockRe   = regexp.MustCompile(`(?s)([^{}]+)\{([^{}]*)\}`)
	commentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	declRe    = regexp.MustCompile(`(--ng-[a-z0-9-]+)\s*:`)

	// Tokens that carry no colour: overriding them per theme would be noise.
	themeNeutral = regexp.MustCompile(`dur|ease|font|mono|radius|sidebar|topbar`)
)

type cssBlock struct {
	selector string
	tokens   map[string]bool
}

func themeBlocks(t *testing.T) []cssBlock {
	t.Helper()
	body, err := os.ReadFile("src/styles/tokens.css")
	if err != nil {
		t.Fatalf("read tokens.css: %v", err)
	}
	stripped := commentRe.ReplaceAllString(string(body), "")

	var out []cssBlock
	for _, m := range blockRe.FindAllStringSubmatch(stripped, -1) {
		tokens := map[string]bool{}
		for _, d := range declRe.FindAllStringSubmatch(m[2], -1) {
			tokens[d[1]] = true
		}
		out = append(out, cssBlock{selector: strings.TrimSpace(m[1]), tokens: tokens})
	}
	return out
}

// :root and [data-theme="light"] have the same specificity, so whichever is
// written last wins. tokens.css used to end with a second :root block placed
// after the light one, which meant fifteen light values were never applied —
// including --ng-fg, the main text colour, which resolved to #f8fafc in light
// mode. Near-white text on a near-white page.
func TestTheLightThemeIsNotOverriddenByADarkBlockBelowIt(t *testing.T) {
	blocks := themeBlocks(t)

	light := -1
	for i, b := range blocks {
		if strings.Contains(b.selector, `data-theme="light"`) {
			light = i
		}
	}
	if light < 0 {
		t.Fatal("tokens.css defines no [data-theme=\"light\"] block")
	}

	for _, b := range blocks[light+1:] {
		if b.selector == ":root" {
			t.Errorf("a :root block follows the light theme block and overrides it "+
				"(equal specificity, later wins); it declares %d tokens. "+
				"Move its declarations into the :root block above the light one.", len(b.tokens))
		}
	}
}

// A colour defined only for dark is painted unchanged on a light page. Every
// colour-bearing token needs a light counterpart, or the light theme inherits
// a value chosen to glow on a dark surface.
func TestEveryColourTokenHasALightCounterpart(t *testing.T) {
	blocks := themeBlocks(t)

	dark, light := map[string]bool{}, map[string]bool{}
	for _, b := range blocks {
		switch {
		case b.selector == ":root":
			for tok := range b.tokens {
				dark[tok] = true
			}
		case strings.Contains(b.selector, `data-theme="light"`):
			for tok := range b.tokens {
				light[tok] = true
			}
		}
	}

	for tok := range dark {
		if light[tok] || themeNeutral.MatchString(tok) {
			continue
		}
		// An alias to another token resolves against whichever theme is
		// active, so it needs no override of its own.
		if isAlias(t, tok) {
			continue
		}
		t.Errorf("%s is defined for dark only; its dark value will be used on a light page", tok)
	}
}

func isAlias(t *testing.T, token string) bool {
	t.Helper()
	body, err := os.ReadFile("src/styles/tokens.css")
	if err != nil {
		return false
	}
	re := regexp.MustCompile(regexp.QuoteMeta(token) + `\s*:\s*var\(`)
	return re.Match(body)
}
