package router

import (
	"context"
	"testing"

	"nabugate/internal/provider"
)

type routerahMediaAdapter struct {
	fakeAdapter
	received string
}

func (a *routerahMediaAdapter) Image(_ context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
	a.received = req.Model
	return provider.ImageResponse{Images: []string{"image"}}, nil
}
func (a *routerahMediaAdapter) Speech(_ context.Context, req provider.SpeechRequest) (provider.SpeechResponse, error) {
	a.received = req.Model
	return provider.SpeechResponse{Audio: []byte("audio"), ContentType: "audio/wav"}, nil
}
func (a *routerahMediaAdapter) Embed(_ context.Context, req provider.EmbeddingRequest) (provider.EmbeddingResponse, error) {
	a.received = req.Model
	return provider.EmbeddingResponse{Embeddings: [][]float64{{0.1, 0.2}}}, nil
}
func (a *routerahMediaAdapter) Transcribe(_ context.Context, req provider.TranscriptionRequest) (provider.TranscriptionResponse, error) {
	a.received = req.Model
	return provider.TranscriptionResponse{Text: "transcribed"}, nil
}

func TestDirectPassthroughForNonChatEndpoints(t *testing.T) {
	a := &routerahMediaAdapter{fakeAdapter: fakeAdapter{name: "routerah"}}
	r := New(map[string]provider.Adapter{"routerah": a}, nil, nil, nil, nil, nil,
		map[string][]string{"routerah": nil}, discardLogger())
	model := "routerah/vendor/nested-model"
	ctx := context.Background()

	if got, err := r.Image(ctx, model, provider.ImageRequest{Prompt: "test"}); err != nil || got.Model != "vendor/nested-model" {
		t.Fatalf("image passthrough: %+v; %v", got, err)
	}
	if got, err := r.Speech(ctx, model, provider.SpeechRequest{Input: "hi"}); err != nil || got.Model != "vendor/nested-model" {
		t.Fatalf("speech passthrough: %+v; %v", got, err)
	}
	if got, err := r.Embed(ctx, model, provider.EmbeddingRequest{Input: []string{"hi"}}); err != nil || got.Model != "vendor/nested-model" {
		t.Fatalf("embedding passthrough: %+v; %v", got, err)
	}
	if got, err := r.Transcribe(ctx, model, provider.TranscriptionRequest{Audio: []byte("voice")}); err != nil || got.Model != "vendor/nested-model" {
		t.Fatalf("transcription passthrough: %+v; %v", got, err)
	}
	if a.received != "vendor/nested-model" {
		t.Fatalf("upstream model ID was altered: %q", a.received)
	}
}

func TestNonChatPassthroughRequiresOptIn(t *testing.T) {
	a := &routerahMediaAdapter{fakeAdapter: fakeAdapter{name: "routerah"}}
	r := New(map[string]provider.Adapter{"routerah": a}, nil, nil, nil, nil, nil, nil, discardLogger())
	if _, err := r.Image(context.Background(), "routerah/image", provider.ImageRequest{}); err == nil {
		t.Fatal("image passthrough must reject non-opted-in providers")
	}
}
