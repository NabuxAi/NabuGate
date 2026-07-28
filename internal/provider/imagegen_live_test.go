package provider

import (
	"context"
	"encoding/base64"
	"os"
	"testing"
)

// Runs only when MRC_IMAGEGEN_KEY is set, so `go test ./...` stays hermetic.
// Its job is the one thing the httptest mock cannot check: that the request
// this adapter builds is one the real service accepts.
func TestLiveRenderAgainstTheRealService(t *testing.T) {
	key := os.Getenv("MRC_IMAGEGEN_KEY")
	if key == "" {
		t.Skip("MRC_IMAGEGEN_KEY not set")
	}
	base := os.Getenv("MRC_IMAGEGEN_URL")
	if base == "" {
		base = "https://imagen-api.nabuxai.com"
	}

	a := NewImagegenAdapter("imagegen", base, key)
	res, err := a.Image(context.Background(), ImageRequest{
		Model:  "nabu-header",
		Prompt: `{"kicker":"هوش مصنوعی","head1":"از دروازه تا تصویر","head2":"یک درخواست، یک هدر","theme":"news","brand":"claude"}`,
	})
	if err != nil {
		t.Fatalf("live render: %v", err)
	}
	if len(res.Images) != 1 {
		t.Fatalf("got %d images", len(res.Images))
	}
	raw, err := base64.StdEncoding.DecodeString(res.Images[0])
	if err != nil {
		t.Fatalf("result is not base64: %v", err)
	}
	if len(raw) < 1000 {
		t.Fatalf("image is only %d bytes — suspiciously small", len(raw))
	}
	if string(raw[1:4]) != "PNG" {
		t.Fatalf("not a PNG: % x", raw[:8])
	}
	t.Logf("rendered %d bytes of PNG", len(raw))
	if out := os.Getenv("MRC_SAVE_TO"); out != "" {
		_ = os.WriteFile(out, raw, 0o644)
	}
}
