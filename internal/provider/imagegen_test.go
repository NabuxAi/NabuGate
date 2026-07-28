package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureRender stands in for mrc_imagegen and records what it was asked for.
func captureRender(t *testing.T, status int, body []byte) (*ImagegenAdapter, *[]renderRequest) {
	t.Helper()
	var got []renderRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/render" {
			t.Errorf("called %s, want /v1/render", r.URL.Path)
		}
		// The service authenticates with X-API-Key; a Bearer token would be
		// silently rejected, so pin the header the adapter actually sends.
		if r.Header.Get("X-API-Key") != "k" {
			t.Errorf("X-API-Key = %q, want %q", r.Header.Get("X-API-Key"), "k")
		}
		var rr renderRequest
		if err := json.NewDecoder(r.Body).Decode(&rr); err != nil {
			t.Errorf("decode body: %v", err)
		}
		got = append(got, rr)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	return NewImagegenAdapter("imagegen", srv.URL, "k"), &got
}

func TestJSONPromptDrivesTheRenderDirectly(t *testing.T) {
	a, got := captureRender(t, 200, []byte("PNGBYTES"))

	prompt := `{"kind":"card","kicker":"خبر فوری","head1":"آنتروپیک جلو زد","head2":"۴۷ میلیارد دلار","theme":"news","brand":"claude","design":3}`
	res, err := a.Image(context.Background(), ImageRequest{Model: "nabu-image", Prompt: prompt})
	if err != nil {
		t.Fatalf("Image: %v", err)
	}

	r := (*got)[0]
	if r.Kind != "card" {
		t.Errorf("kind = %q, want card — the JSON must win over the model name", r.Kind)
	}
	if r.Head1 != "آنتروپیک جلو زد" || r.Head2 != "۴۷ میلیارد دلار" {
		t.Errorf("headlines not passed through: %+v", r)
	}
	if r.Kicker != "خبر فوری" || r.Theme != "news" || r.Brand != "claude" {
		t.Errorf("fields not passed through: %+v", r)
	}
	if len(res.Images) != 1 {
		t.Fatalf("got %d images, want 1", len(res.Images))
	}
	if decoded, _ := base64.StdEncoding.DecodeString(res.Images[0]); string(decoded) != "PNGBYTES" {
		t.Errorf("image bytes were not returned base64-encoded")
	}
}

func TestPlainPromptIsSplitAcrossTwoLines(t *testing.T) {
	a, got := captureRender(t, 200, []byte("x"))

	// Long enough that one line would overflow the canvas.
	_, err := a.Image(context.Background(), ImageRequest{
		Prompt: "هوش مصنوعی چطور کار می‌کند و چرا باید یادش بگیریم",
	})
	if err != nil {
		t.Fatalf("Image: %v", err)
	}

	r := (*got)[0]
	if r.Head1 == "" || r.Head2 == "" {
		t.Fatalf("expected the headline split across two lines, got %+v", r)
	}
	// The split should land near the middle, not leave one line nearly empty —
	// that is the difference between a designed layout and a wrapped string.
	l1, l2 := len([]rune(r.Head1)), len([]rune(r.Head2))
	if l1 == 0 || l2 == 0 || l1 > l2*3 || l2 > l1*3 {
		t.Errorf("lines are lopsided: %d vs %d (%q / %q)", l1, l2, r.Head1, r.Head2)
	}
}

func TestExplicitNewlineIsRespected(t *testing.T) {
	a, got := captureRender(t, 200, []byte("x"))

	_, err := a.Image(context.Background(), ImageRequest{Prompt: "خط اول\nخط دوم"})
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	// A caller who wrote the break chose where the line ends; do not re-balance.
	if r := (*got)[0]; r.Head1 != "خط اول" || r.Head2 != "خط دوم" {
		t.Errorf("newline not honoured: %+v", r)
	}
}

func TestShortPromptStaysOnOneLine(t *testing.T) {
	a, got := captureRender(t, 200, []byte("x"))

	if _, err := a.Image(context.Background(), ImageRequest{Prompt: "سلام"}); err != nil {
		t.Fatalf("Image: %v", err)
	}
	if r := (*got)[0]; r.Head2 != "" {
		t.Errorf("a short headline was forced onto two lines: %+v", r)
	}
}

func TestKindComesFromTheModelName(t *testing.T) {
	for model, want := range map[string]string{
		"nabu-card":   "card",
		"nabu-story":  "story",
		"nabu-header": "header",
		"nabu-design": "design",
		"something":   "header", // 16:9 is the sane default for an article
	} {
		a, got := captureRender(t, 200, []byte("x"))
		if _, err := a.Image(context.Background(), ImageRequest{Model: model, Prompt: "t"}); err != nil {
			t.Fatalf("%s: %v", model, err)
		}
		if r := (*got)[0]; r.Kind != want {
			t.Errorf("model %q -> kind %q, want %q", model, r.Kind, want)
		}
	}
}

func TestPortraitRequestPicksTheCard(t *testing.T) {
	a, got := captureRender(t, 200, []byte("x"))
	if _, err := a.Image(context.Background(), ImageRequest{Prompt: "t", Size: "1080x1350"}); err != nil {
		t.Fatal(err)
	}
	if r := (*got)[0]; r.Kind != "card" {
		t.Errorf("a portrait size should select the card canvas, got %q", r.Kind)
	}
}

func TestBatchVariesTheLayout(t *testing.T) {
	a, got := captureRender(t, 200, []byte("x"))

	if _, err := a.Image(context.Background(), ImageRequest{Model: "nabu-header", Prompt: "t", N: 3}); err != nil {
		t.Fatalf("Image: %v", err)
	}
	if len(*got) != 3 {
		t.Fatalf("made %d renders, want 3", len(*got))
	}
	// Three identical files would be a useless answer to "give me three".
	seen := map[int]bool{}
	for _, r := range *got {
		if seen[r.Variant] {
			t.Fatalf("variant %d repeated across the batch", r.Variant)
		}
		seen[r.Variant] = true
	}
}

func TestOverlongTextIsClippedNotSentWhole(t *testing.T) {
	a, got := captureRender(t, 200, []byte("x"))

	long := strings.Repeat("ا", 400)
	body, _ := json.Marshal(map[string]string{"head1": long, "kicker": long})
	if _, err := a.Image(context.Background(), ImageRequest{Prompt: string(body)}); err != nil {
		t.Fatal(err)
	}

	r := (*got)[0]
	// Past the canvas width the renderer collides with its own frame, so the
	// cut belongs here where the limit is written down.
	if n := len([]rune(r.Head1)); n > maxHeadLn {
		t.Errorf("head1 is %d runes, over the %d limit", n, maxHeadLn)
	}
	if n := len([]rune(r.Kicker)); n > maxKicker {
		t.Errorf("kicker is %d runes, over the %d limit", n, maxKicker)
	}
}

func TestClipCountsRunesNotBytes(t *testing.T) {
	// Persian is multi-byte; a byte limit would cut a character in half and
	// produce mojibake in the rendered image.
	out := clip(strings.Repeat("ش", 100), 10)
	if n := len([]rune(out)); n > 10 {
		t.Errorf("clip returned %d runes, want <= 10", n)
	}
	if !strings.HasPrefix(out, "ش") {
		t.Errorf("clip corrupted the text: %q", out)
	}
}

func TestServiceErrorSurfacesItsMessage(t *testing.T) {
	a, _ := captureRender(t, 400, []byte(`{"detail":"scale 8 exceeds the pixel cap."}`))

	_, err := a.Image(context.Background(), ImageRequest{Prompt: "t"})
	if err == nil {
		t.Fatal("expected an error")
	}
	// A bare status code sends the caller to the wrong place; the service knows
	// what was wrong and says so.
	if !strings.Contains(err.Error(), "pixel cap") {
		t.Errorf("error lost the service's message: %v", err)
	}
}

func TestEmptyBodyIsAnError(t *testing.T) {
	a, _ := captureRender(t, 200, nil)

	// A 200 with no bytes would otherwise become a valid-looking blank image,
	// indistinguishable from a design that is meant to be sparse.
	if _, err := a.Image(context.Background(), ImageRequest{Prompt: "t"}); err == nil {
		t.Fatal("an empty 200 should not be reported as a successful image")
	}
}

func TestMalformedJSONFallsBackToText(t *testing.T) {
	a, got := captureRender(t, 200, []byte("x"))

	// Prose that merely starts with a brace should still produce an image
	// rather than a parse error.
	if _, err := a.Image(context.Background(), ImageRequest{Prompt: "{این یک متن است"}); err != nil {
		t.Fatalf("Image: %v", err)
	}
	if r := (*got)[0]; r.Head1 == "" {
		t.Errorf("fell through to an empty render: %+v", r)
	}
}

func TestChatIsRefused(t *testing.T) {
	a := NewImagegenAdapter("imagegen", "http://example.invalid", "k")
	if _, err := a.Chat(context.Background(), ChatRequest{}); err == nil {
		t.Error("chat should fail cleanly on an image-only provider")
	}
}
