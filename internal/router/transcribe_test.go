package router

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// transcriber records what it was asked and answers as told.
type transcriber struct {
	name  string
	text  string
	err   error
	calls *int
	audio *[]byte
}

func (t transcriber) Name() string { return t.name }
func (t transcriber) Chat(context.Context, provider.ChatRequest) (provider.ChatResponse, error) {
	return provider.ChatResponse{}, errors.New("not used")
}
func (t transcriber) Transcribe(_ context.Context, req provider.TranscriptionRequest) (provider.TranscriptionResponse, error) {
	if t.calls != nil {
		*t.calls++
	}
	if t.audio != nil {
		*t.audio = req.Audio
	}
	if t.err != nil {
		return provider.TranscriptionResponse{}, t.err
	}
	return provider.TranscriptionResponse{
		Text:     t.text,
		Language: "fa",
		Duration: 7,
		Segments: []provider.TranscriptionSegment{{ID: 0, Start: 0, End: 7, Text: t.text}},
	}, nil
}

// chatOnly supports no transcription at all.
type chatOnly struct{ name string }

func (c chatOnly) Name() string { return c.name }
func (c chatOnly) Chat(context.Context, provider.ChatRequest) (provider.ChatResponse, error) {
	return provider.ChatResponse{}, errors.New("not used")
}

func routeWith(primary config.Target, fallbacks ...config.Target) map[string]config.ModelRoute {
	return map[string]config.ModelRoute{"nabu-transcribe": {Primary: primary, Fallback: fallbacks}}
}

func TestTranscribeUsesPrimary(t *testing.T) {
	adapters := map[string]provider.Adapter{"a": transcriber{name: "a", text: "spoken words"}}
	r := New(adapters, nil, nil, nil, nil,
		routeWith(config.Target{Provider: "a", Model: "whisper-1"}), nil, discardLogger())

	res, err := r.Transcribe(context.Background(), "nabu-transcribe",
		provider.TranscriptionRequest{Audio: []byte("x"), Filename: "a.wav"})
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if res.Text != "spoken words" || res.Provider != "a" || res.Model != "whisper-1" {
		t.Errorf("got %+v", res)
	}
	if len(res.Segments) != 1 || res.Segments[0].End != 7 {
		t.Errorf("segments = %+v", res.Segments)
	}
}

func TestTranscribeFallsOverToTheNextVendor(t *testing.T) {
	var firstCalls, secondCalls int
	adapters := map[string]provider.Adapter{
		"down": transcriber{name: "down", err: errors.New("502"), calls: &firstCalls},
		"up":   transcriber{name: "up", text: "rescued", calls: &secondCalls},
	}
	r := New(adapters, nil, nil, nil, nil,
		routeWith(config.Target{Provider: "down", Model: "m1"},
			config.Target{Provider: "up", Model: "m2"}), nil, discardLogger())

	res, err := r.Transcribe(context.Background(), "nabu-transcribe",
		provider.TranscriptionRequest{Audio: []byte("x"), Filename: "a.wav"})
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if res.Text != "rescued" || res.Provider != "up" {
		t.Errorf("got %+v", res)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Errorf("calls: first=%d second=%d", firstCalls, secondCalls)
	}
}

func TestTranscribeReplaysTheAudioToTheFallback(t *testing.T) {
	// The bytes are held across attempts on purpose: a fallback handed a
	// consumed stream could not retry at all.
	var seen []byte
	adapters := map[string]provider.Adapter{
		"down": transcriber{name: "down", err: errors.New("502")},
		"up":   transcriber{name: "up", text: "ok", audio: &seen},
	}
	r := New(adapters, nil, nil, nil, nil,
		routeWith(config.Target{Provider: "down", Model: "m1"},
			config.Target{Provider: "up", Model: "m2"}), nil, discardLogger())

	if _, err := r.Transcribe(context.Background(), "nabu-transcribe",
		provider.TranscriptionRequest{Audio: []byte("RIFFdata"), Filename: "a.wav"}); err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if string(seen) != "RIFFdata" {
		t.Errorf("fallback received %q", seen)
	}
}

func TestTranscribeSkipsAProviderThatCannotTranscribe(t *testing.T) {
	adapters := map[string]provider.Adapter{
		"chatonly": chatOnly{name: "chatonly"},
		"real":     transcriber{name: "real", text: "words"},
	}
	r := New(adapters, nil, nil, nil, nil,
		routeWith(config.Target{Provider: "chatonly", Model: "m1"},
			config.Target{Provider: "real", Model: "m2"}), nil, discardLogger())

	res, err := r.Transcribe(context.Background(), "nabu-transcribe",
		provider.TranscriptionRequest{Audio: []byte("x"), Filename: "a.wav"})
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if res.Provider != "real" {
		t.Errorf("provider = %s", res.Provider)
	}
}

func TestTranscribeUnknownAlias(t *testing.T) {
	r := New(map[string]provider.Adapter{}, nil, nil, nil, nil, nil, nil, discardLogger())
	_, err := r.Transcribe(context.Background(), "nope",
		provider.TranscriptionRequest{Audio: []byte("x")})
	if err == nil || !strings.Contains(err.Error(), "unknown transcription alias") {
		t.Fatalf("err = %v", err)
	}
}

func TestTranscribeAllTargetsFailedNamesTheAlias(t *testing.T) {
	adapters := map[string]provider.Adapter{
		"a": transcriber{name: "a", err: errors.New("boom")},
	}
	r := New(adapters, nil, nil, nil, nil,
		routeWith(config.Target{Provider: "a", Model: "m"}), nil, discardLogger())

	_, err := r.Transcribe(context.Background(), "nabu-transcribe",
		provider.TranscriptionRequest{Audio: []byte("x")})
	if err == nil || !strings.Contains(err.Error(), "all targets failed") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "nabu-transcribe") {
		t.Errorf("error should name the alias: %v", err)
	}
}

func TestTranscribeSkipsAMissingProvider(t *testing.T) {
	// A provider whose key env is empty is simply absent; the chain must
	// continue rather than fail on the gap.
	adapters := map[string]provider.Adapter{"present": transcriber{name: "present", text: "ok"}}
	r := New(adapters, nil, nil, nil, nil,
		routeWith(config.Target{Provider: "absent", Model: "m1"},
			config.Target{Provider: "present", Model: "m2"}), nil, discardLogger())

	res, err := r.Transcribe(context.Background(), "nabu-transcribe",
		provider.TranscriptionRequest{Audio: []byte("x")})
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if res.Provider != "present" {
		t.Errorf("provider = %s", res.Provider)
	}
}
