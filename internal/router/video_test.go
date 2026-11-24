package router

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

type videoRung struct {
	fakeAdapter
	err   error
	calls *[]string
}

func (v videoRung) CreateVideo(_ context.Context, req provider.VideoRequest) (provider.VideoJob, error) {
	*v.calls = append(*v.calls, v.name)
	if v.err != nil {
		return provider.VideoJob{}, v.err
	}
	return provider.VideoJob{TaskID: "task-" + v.name, Model: req.Model, Resolution: "720P", Seconds: 5}, nil
}

func (v videoRung) VideoStatus(_ context.Context, taskID string) (provider.VideoStatus, error) {
	return provider.VideoStatus{State: provider.VideoProcessing}, nil
}

func videoRouter(adapters map[string]provider.Adapter, route config.ModelRoute) *Router {
	r := New(adapters, nil, nil, nil, nil, nil, nil, discardLogger())
	r.SetVideos(map[string]config.ModelRoute{"nabu-video": route})
	r.SetCallerAdapter(func(name, key string) (provider.Adapter, bool) {
		a, ok := adapters[name]
		return a, ok
	})
	return r
}

// Starting a video falls back past an outage, and the job remembers who has it.
func TestVideoStartFallsBackAndNamesTheVendor(t *testing.T) {
	var calls []string
	adapters := map[string]provider.Adapter{
		"a": videoRung{fakeAdapter: fakeAdapter{name: "a"}, err: errors.New("503"), calls: &calls},
		"b": videoRung{fakeAdapter: fakeAdapter{name: "b"}, calls: &calls},
	}
	r := videoRouter(adapters, config.ModelRoute{
		Primary:  config.Target{Provider: "a", Model: "m1"},
		Fallback: []config.Target{{Provider: "b", Model: "m2"}},
	})
	res, err := r.Video(context.Background(), "nabu-video", provider.VideoRequest{Prompt: "x"})
	if err != nil || res.Provider != "b" || res.Model != "m2" || res.Job.TaskID != "task-b" || strings.Join(calls, ",") != "a,b" {
		t.Fatalf("res %+v, err %v, calls %v", res, err, calls)
	}
	if st, err := r.VideoStatus(context.Background(), "b", "task-b"); err != nil || st.State != provider.VideoProcessing {
		t.Fatalf("poll: %+v %v", st, err)
	}
	if _, err := r.VideoStatus(context.Background(), "missing", "t"); err == nil {
		t.Fatal("a poll on a provider that is not up must fail")
	}
}

// A malformed request is not retried elsewhere.
func TestVideoInvalidRequestEndsTheChain(t *testing.T) {
	var calls []string
	adapters := map[string]provider.Adapter{
		"a": videoRung{fakeAdapter: fakeAdapter{name: "a"}, err: fmt.Errorf("%w: bad size", provider.ErrInvalidVideoRequest), calls: &calls},
		"b": videoRung{fakeAdapter: fakeAdapter{name: "b"}, calls: &calls},
	}
	r := videoRouter(adapters, config.ModelRoute{
		Primary:  config.Target{Provider: "a", Model: "m1"},
		Fallback: []config.Target{{Provider: "b", Model: "m2"}},
	})
	_, err := r.Video(context.Background(), "nabu-video", provider.VideoRequest{Prompt: "x"})
	if !errors.Is(err, provider.ErrInvalidVideoRequest) || len(calls) != 1 {
		t.Fatalf("err %v, calls %v", err, calls)
	}
}

// A caller's own key cannot start a job it could not finish; own-first falls
// to the gateway's key, own alone says why it did nothing.
func TestVideoSkipsCallerKeys(t *testing.T) {
	var calls []string
	adapters := map[string]provider.Adapter{"a": videoRung{fakeAdapter: fakeAdapter{name: "a"}, calls: &calls}}
	r := videoRouter(adapters, config.ModelRoute{Primary: config.Target{Provider: "a", Model: "m1"}})

	ownFirst := context.WithValue(context.Background(), CallerKeysCtxKey{}, CallerKeys{Keys: map[string][]string{"a": {"k"}}, Mode: ModeOwnFirst})
	if res, err := r.Video(ownFirst, "nabu-video", provider.VideoRequest{Prompt: "x"}); err != nil || res.Provider != "a" {
		t.Fatalf("own-first: %+v %v", res, err)
	}
	own := context.WithValue(context.Background(), CallerKeysCtxKey{}, CallerKeys{Keys: map[string][]string{"a": {"k"}}, Mode: ModeOwn})
	if _, err := r.Video(own, "nabu-video", provider.VideoRequest{Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "your own key cannot start a video job") {
		t.Fatalf("own: %v", err)
	}
	if _, err := r.Video(context.Background(), "nope", provider.VideoRequest{}); err == nil || !strings.HasPrefix(err.Error(), "unknown video alias") {
		t.Fatalf("unknown alias: %v", err)
	}
}

func TestVideoAliasesAreListed(t *testing.T) {
	var calls []string
	r := videoRouter(map[string]provider.Adapter{"a": videoRung{fakeAdapter: fakeAdapter{name: "a"}, calls: &calls}},
		config.ModelRoute{Primary: config.Target{Provider: "a", Model: "m1"}})
	for _, info := range r.AliasInfos() {
		if info.ID == "nabu-video" && info.Owner == "a" {
			if kind, _ := r.RouteKind("nabu-video"); kind != "videos" {
				t.Fatalf("RouteKind = %q", kind)
			}
			return
		}
	}
	t.Fatalf("nabu-video not in %+v", r.AliasInfos())
}

// pairRung records the request a video rung was handed.
type pairRung struct {
	fakeAdapter
	got *provider.VideoRequest
}

func (p pairRung) CreateVideo(_ context.Context, req provider.VideoRequest) (provider.VideoJob, error) {
	*p.got = req
	return provider.VideoJob{TaskID: "t", Model: req.Model}, nil
}

func (p pairRung) VideoStatus(context.Context, string) (provider.VideoStatus, error) {
	return provider.VideoStatus{State: provider.VideoProcessing}, nil
}

// A rung's image_model travels with the request, so the adapter can run it
// when the request carries a first frame.
func TestVideoHandsTheRungsImageModelToTheAdapter(t *testing.T) {
	var got provider.VideoRequest
	r := videoRouter(map[string]provider.Adapter{"a": pairRung{fakeAdapter: fakeAdapter{name: "a"}, got: &got}},
		config.ModelRoute{Primary: config.Target{Provider: "a", Model: "wan2.1-t2v-turbo", ImageModel: "wan2.2-i2v-flash"}})
	if _, err := r.Video(context.Background(), "nabu-video", provider.VideoRequest{Prompt: "x", ImageURL: "https://e.com/f.png"}); err != nil {
		t.Fatal(err)
	}
	if got.Model != "wan2.1-t2v-turbo" || got.ImageModel != "wan2.2-i2v-flash" {
		t.Fatalf("adapter got model %q, image model %q", got.Model, got.ImageModel)
	}
}
