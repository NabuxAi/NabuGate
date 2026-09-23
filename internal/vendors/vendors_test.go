package vendors

import (
	"strings"
	"testing"
	"unicode"
)

// Every row on the catalogue screen must render as something deliberate. A
// vendor needs a colour so its tile is tinted, and an icon path that is
// actually a path — a malformed `d` draws nothing at all and fails silently,
// which looks identical to a vendor we simply have no mark for.
func TestEveryVendorRendersAsSomething(t *testing.T) {
	for _, v := range Known() {
		if v.Label == "" {
			t.Errorf("%s: no label; the tile would show its config name", v.Name)
		}
		if v.Color == "" {
			t.Errorf("%s: no colour; the tile falls back to a grey lettermark", v.Name)
		} else if !strings.HasPrefix(v.Color, "#") || (len(v.Color) != 7 && len(v.Color) != 4) {
			t.Errorf("%s: colour %q is not a hex value", v.Name, v.Color)
		}
		if v.Icon != "" {
			if !strings.HasPrefix(v.Icon, "M") && !strings.HasPrefix(v.Icon, "m") {
				t.Errorf("%s: icon does not start with a moveto, so it draws nothing", v.Name)
			}
			if strings.ContainsAny(v.Icon, "<>\"") {
				t.Errorf("%s: icon contains markup; it is a path, not an element", v.Name)
			}
		}
		if len(v.Capabilities) == 0 {
			t.Errorf("%s: no capabilities, so it matches no filter on the screen", v.Name)
		}
	}
}

// Lookup is how the console decides between a mark and a lettermark, and Get
// must never fail — a provider in the config that nobody catalogued still has
// to render.
func TestGetInventsAnEntryForTheUncatalogued(t *testing.T) {
	v := Get("somethingnobodyadded")
	if v.Name != "somethingnobodyadded" || v.Label != "somethingnobodyadded" {
		t.Errorf("Get = %+v", v)
	}
	if _, ok := Lookup("somethingnobodyadded"); ok {
		t.Error("Lookup claimed to know it")
	}

	// Case and spacing come from a config file and a header, so neither can be
	// allowed to decide whether a vendor is found.
	if _, ok := Lookup("  Gemini  "); !ok {
		t.Error("Lookup is case- or space-sensitive")
	}
	// The name echoed back is the caller's spelling, since that is what the
	// config and the router use.
	if got := Get("Gemini").Name; got != "Gemini" {
		t.Errorf("Get(%q).Name = %q", "Gemini", got)
	}
}

// The console switches to LabelEn and BlurbEn on its English screens and falls
// back to the Persian pair when they are empty. So a missing translation does
// not fail anywhere a person would notice: it puts a Persian sentence, or a
// name in a script the reader cannot even sound out, on a screen that is
// otherwise English. Caught here rather than by someone switching the language.
func TestEveryVendorReadsInEnglish(t *testing.T) {
	for _, v := range Known() {
		if v.Blurb != "" && strings.TrimSpace(v.BlurbEn) == "" {
			t.Errorf("%s: has a Blurb but no BlurbEn", v.Name)
		}
		if hasArabicScript(v.Label) && strings.TrimSpace(v.LabelEn) == "" {
			t.Errorf("%s: label %q is in Persian script but has no LabelEn", v.Name, v.Label)
		}
		// The likeliest slip is pasting the Persian into the English field,
		// which passes both checks above.
		if hasArabicScript(v.LabelEn) || hasArabicScript(v.BlurbEn) {
			t.Errorf("%s: the English label or blurb is not in English", v.Name)
		}
	}
}

func hasArabicScript(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Arabic, r) {
			return true
		}
	}
	return false
}
