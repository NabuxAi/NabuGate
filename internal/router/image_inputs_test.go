package router

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// imageRung is an image adapter that records what it was asked. editor marks
// it as one that reads input pictures; err makes it fail.
type imageRung struct {
	fakeAdapter
	editor bool
	err    error
	calls  *[]string
	got    *provider.ImageRequest
}

func (a imageRung) Image(_ context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
	*a.calls = append(*a.calls, a.name)
	if a.got != nil {
		*a.got = req
	}
	if a.err != nil {
		return provider.ImageResponse{}, a.err
	}
	return provider.ImageResponse{Images: []string{"from-" + a.name}}, nil
}

// editorRung adds the ImageInputAdapter marker to imageRung.
type editorRung struct{ imageRung }

func (editorRung) AcceptsInputImages(model string) bool { return !strings.Contains(model, "t2i") }

func imageRouter(adapters map[string]provider.Adapter, route config.ModelRoute) *Router {
	return New(adapters, nil, map[string]config.ModelRoute{"edit": route}, nil, nil, nil, nil, discardLogger())
}

// An edit skips every rung that cannot read input pictures — a text-only
// model, a stock-photo search — and lands on the one that can.
func TestImageEditSkipsRungsWithoutInputImages(t *testing.T) {
	var calls []string
	var got provider.ImageRequest
	adapters := map[string]provider.Adapter{
		"openai":          imageRung{fakeAdapter: fakeAdapter{name: "openai"}, calls: &calls},
		"pexels":          imageRung{fakeAdapter: fakeAdapter{name: "pexels"}, calls: &calls},
		"dashscope-image": editorRung{imageRung{fakeAdapter: fakeAdapter{name: "dashscope-image"}, calls: &calls, got: &got}},
	}
	route := config.ModelRoute{
		Primary: config.Target{Provider: "openai", Model: "gpt-image-1"},
		Fallback: []config.Target{
			{Provider: "dashscope-image", Model: "wan2.6-t2i"},
			{Provider: "pexels", Model: "search"},
			{Provider: "dashscope-image", Model: "wan2.6-image"},
		},
	}
	res, err := imageRouter(adapters, route).Image(context.Background(), "edit",
		provider.ImageRequest{Prompt: "watercolor", Images: []string{"https://example.com/a.png"}})
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	if res.Model != "wan2.6-image" || len(calls) != 1 || calls[0] != "dashscope-image" {
		t.Fatalf("served by %s/%s after calls %v; want only dashscope-image/wan2.6-image", res.Provider, res.Model, calls)
	}
	if len(got.Images) != 1 || got.Images[0] != "https://example.com/a.png" {
		t.Fatalf("input images did not reach the adapter: %v", got.Images)
	}
}

// A failing editor must not fall through to an unrelated picture: the chain
// fails, and every skipped rung says why.
func TestImageEditNeverFallsThroughToAnUnrelatedPicture(t *testing.T) {
	var calls []string
	adapters := map[string]provider.Adapter{
		"dashscope-image": editorRung{imageRung{fakeAdapter: fakeAdapter{name: "dashscope-image"}, calls: &calls, err: errors.New("upstream 500")}},
		"pexels":          imageRung{fakeAdapter: fakeAdapter{name: "pexels"}, calls: &calls},
	}
	route := config.ModelRoute{
		Primary:  config.Target{Provider: "dashscope-image", Model: "wan2.6-image"},
		Fallback: []config.Target{{Provider: "pexels", Model: "search"}},
	}
	_, err := imageRouter(adapters, route).Image(context.Background(), "edit",
		provider.ImageRequest{Prompt: "x", Images: []string{"https://example.com/a.png"}})
	if err == nil {
		t.Fatal("an edit whose only editor failed was served anyway")
	}
	if len(calls) != 1 || calls[0] != "dashscope-image" {
		t.Fatalf("calls = %v; pexels must never be asked for an edit", calls)
	}
	if !strings.Contains(err.Error(), "pexels/search: does not accept input images") || !strings.Contains(err.Error(), "upstream 500") {
		t.Fatalf("error does not record the chain: %v", err)
	}
	if errors.Is(err, ErrNoImageInput) {
		t.Fatalf("an upstream failure is not a request error: %v", err)
	}
}

// An alias with nothing that edits is the caller's mistake, not an outage.
func TestImageEditOnATextOnlyAliasIsARequestError(t *testing.T) {
	var calls []string
	adapters := map[string]provider.Adapter{
		"openai":          imageRung{fakeAdapter: fakeAdapter{name: "openai"}, calls: &calls},
		"dashscope-image": editorRung{imageRung{fakeAdapter: fakeAdapter{name: "dashscope-image"}, calls: &calls}},
	}
	route := config.ModelRoute{
		Primary:  config.Target{Provider: "openai", Model: "gpt-image-1"},
		Fallback: []config.Target{{Provider: "dashscope-image", Model: "wan2.6-t2i"}},
	}
	_, err := imageRouter(adapters, route).Image(context.Background(), "edit",
		provider.ImageRequest{Prompt: "x", Images: []string{"https://example.com/a.png"}})
	if !errors.Is(err, ErrNoImageInput) {
		t.Fatalf("err = %v; want ErrNoImageInput", err)
	}
	if len(calls) != 0 {
		t.Fatalf("calls = %v; no rung should have been asked", calls)
	}
}

// Without input images nothing changes: the first rung serves, editor or not.
func TestImageWithoutInputsKeepsTheChain(t *testing.T) {
	var calls []string
	adapters := map[string]provider.Adapter{
		"openai":          imageRung{fakeAdapter: fakeAdapter{name: "openai"}, calls: &calls},
		"dashscope-image": editorRung{imageRung{fakeAdapter: fakeAdapter{name: "dashscope-image"}, calls: &calls}},
	}
	route := config.ModelRoute{
		Primary:  config.Target{Provider: "openai", Model: "gpt-image-1"},
		Fallback: []config.Target{{Provider: "dashscope-image", Model: "wan2.6-image"}},
	}
	res, err := imageRouter(adapters, route).Image(context.Background(), "edit", provider.ImageRequest{Prompt: "x"})
	if err != nil || res.Provider != "openai" || len(calls) != 1 {
		t.Fatalf("served by %q (calls %v, err %v); want openai first", res.Provider, calls, err)
	}
}
