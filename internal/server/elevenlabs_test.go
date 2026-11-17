package server

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

func elevenPrices() map[string]usage.Price {
	return map[string]usage.Price{
		"eleven_v3":                   {PerMillionChars: 80},
		"eleven_multilingual_v2":      {PerMillionChars: 80},
		"scribe_v2":                   {PerMinute: 0.003667},
		"elevenlabs/scribe_v2":        {PerMinute: 0.003667},
		"elevenlabs/dubbing":          {PerMinute: 0.50},
		"elevenlabs/dubbing_v2":       {PerMinute: 2.20},
		"sync-lipsync-v3":             {PerMinute: 7.20},
		"eleven_text_to_sound_v2":     {PerMinute: 0.12},
		"music_v1":                    {PerMinute: 0.15},
		"eleven_multilingual_sts_v2":  {PerMinute: 0.12},
		"elevenlabs/forced-alignment": {PerMinute: 0.003333},
	}
}

func elevenGateway(t *testing.T, upstream http.HandlerFunc, keys *policy.Enforcer) *httptest.Server {
	t.Helper()
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	adapters := map[string]provider.Adapter{
		"elevenlabs": provider.NewElevenLabsAdapter("elevenlabs", up.URL, "gateway-secret"),
	}
	r := router.New(adapters, nil, nil, nil, nil, nil, nil, discardLogger())
	if keys == nil {
		keys = policy.New(nil, nil)
	}
	srv := New(r, keys, usage.New(elevenPrices()), nil, discardLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestElevenLabsProxyForwardsSpeechBodyAndHidesTheKey(t *testing.T) {
	var gotPath, gotQuery, gotKey, gotAuth, gotClientKey, gotDuration string
	var gotBody []byte
	ts := elevenGateway(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotKey = r.Header.Get("xi-api-key")
		gotAuth = r.Header.Get("Authorization")
		gotClientKey = r.Header.Get("X-Client-Marker")
		gotDuration = r.Header.Get("X-Nabu-Duration-Seconds")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("AUDIO"))
	}, nil)

	body := `{"text":"hi","model_id":"eleven_v3","language_code":"fa","voice_settings":{"stability":0.3,"similarity_boost":0.8}}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/elevenlabs/v1/text-to-speech/21m00Tcm4TlvDq8ikWAM/with-timestamps?output_format=mp3_44100_128", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", "caller-should-not-pass")
	req.Header.Set("Authorization", "Bearer client-token")
	req.Header.Set("X-Client-Marker", "keep")
	req.Header.Set("X-Nabu-Duration-Seconds", "4")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	audio, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if string(audio) != "AUDIO" || resp.Header.Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("body %q type %q", audio, resp.Header.Get("Content-Type"))
	}
	if gotPath != "/v1/text-to-speech/21m00Tcm4TlvDq8ikWAM/with-timestamps" || gotQuery != "output_format=mp3_44100_128" {
		t.Fatalf("upstream url %s?%s", gotPath, gotQuery)
	}
	if gotKey != "gateway-secret" || gotAuth != "" || gotDuration != "" {
		t.Fatalf("key leaked or replaced wrong: xi=%q auth=%q duration=%q", gotKey, gotAuth, gotDuration)
	}
	if gotClientKey != "keep" {
		t.Fatalf("ordinary header dropped: %q", gotClientKey)
	}
	if string(gotBody) != body {
		t.Fatalf("body rewritten: %s", gotBody)
	}
	if resp.Header.Get("X-Nabu-Provider") != "elevenlabs" || resp.Header.Get("X-Nabu-Model") != "eleven_v3" {
		t.Fatalf("headers: %+v", resp.Header)
	}
	// 2 characters at $80 / 1M.
	if resp.Header.Get("X-Nabu-Cost-USD") != "0.000160" {
		t.Fatalf("cost %q", resp.Header.Get("X-Nabu-Cost-USD"))
	}
	if resp.Header.Get("xi-api-key") != "" {
		t.Fatal("upstream key was echoed")
	}
}

func TestElevenLabsProxyPassesUpstreamErrorsThrough(t *testing.T) {
	const upstreamBody = `{"detail":{"status":"invalid_request","message":"voice not found"}}`
	ts := elevenGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, upstreamBody)
	}, nil)
	resp, err := http.Post(ts.URL+"/elevenlabs/v1/text-to-speech/missing", "application/json", strings.NewReader(`{"text":"hi","model_id":"eleven_v3"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if string(got) != upstreamBody {
		t.Fatalf("error body wrapped: %s", got)
	}
	if resp.Header.Get("X-Nabu-Cost-USD") != "0.000000" {
		t.Fatalf("failed call was billed: %s", resp.Header.Get("X-Nabu-Cost-USD"))
	}
}

func TestElevenLabsProxyStreamsMultipartAndAudio(t *testing.T) {
	var gotCT, gotPath string
	var gotFile []byte
	ts := elevenGateway(t, func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		f, _, err := r.FormFile("audio")
		if err != nil {
			t.Errorf("form file: %v", err)
			http.Error(w, "no file", 400)
			return
		}
		gotFile, _ = io.ReadAll(f)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("CHANGED"))
	}, nil)

	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	part, _ := form.CreateFormFile("audio", "in.wav")
	_, _ = part.Write([]byte("WAVDATA"))
	_ = form.WriteField("model_id", "eleven_multilingual_sts_v2")
	_ = form.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/elevenlabs/v1/speech-to-speech/voice1", &buf)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("X-Nabu-Duration-Seconds", "30")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if string(out) != "CHANGED" || string(gotFile) != "WAVDATA" {
		t.Fatalf("file %q out %q", gotFile, out)
	}
	if !strings.Contains(gotCT, "multipart/form-data") {
		t.Fatalf("content-type %q", gotCT)
	}
	if gotPath != "/v1/voice-changer/voice1" {
		t.Fatalf("speech-to-speech was not rewritten: %s", gotPath)
	}
	// The alias path is the voice changer.
	// 30 seconds at $0.12 / minute = $0.06.
	if resp.Header.Get("X-Nabu-Cost-USD") != "0.060000" {
		t.Fatalf("cost %q model %q", resp.Header.Get("X-Nabu-Cost-USD"), resp.Header.Get("X-Nabu-Model"))
	}
}

func TestElevenLabsProxyRefusesAccountRoutes(t *testing.T) {
	called := false
	ts := elevenGateway(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}, nil)
	resp, err := http.Get(ts.URL + "/elevenlabs/v1/user")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if called {
		t.Fatal("account route was forwarded")
	}
}

func TestElevenLabsProxyRequiresTheProjectKeyAndItsAllowList(t *testing.T) {
	keys := policy.New(nil, []policy.KeyConfig{
		{Key: "wide", Project: "nabugen", Allow: []string{"elevenlabs", "elevenlabs/*"}, RateLimit: 1},
		{Key: "narrow", Project: "other", Allow: []string{"nabu-fast"}},
	})
	hits := 0
	ts := elevenGateway(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"voices":[]}`)
	}, keys)

	resp, err := http.Get(ts.URL + "/elevenlabs/v1/voices")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing key: %d", resp.StatusCode)
	}

	narrow, _ := http.NewRequest(http.MethodGet, ts.URL+"/elevenlabs/v1/voices", nil)
	narrow.Header.Set("Authorization", "Bearer narrow")
	resp, err = http.DefaultClient.Do(narrow)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("narrow key: %d", resp.StatusCode)
	}

	wide, _ := http.NewRequest(http.MethodGet, ts.URL+"/elevenlabs/v1/voices", nil)
	wide.Header.Set("Authorization", "Bearer wide")
	resp, err = http.DefaultClient.Do(wide)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("wide key: %d", resp.StatusCode)
	}
	// The same key's rate limit is one request per minute.
	resp, err = http.DefaultClient.Do(wide)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("rate limit: %d", resp.StatusCode)
	}
	if hits != 1 {
		t.Fatalf("upstream hits %d, want 1", hits)
	}
}

func TestElevenLabsProxyBillsDubbingOnceAndLipsyncFromDuration(t *testing.T) {
	ts := elevenGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/dubbing":
			_, _ = io.WriteString(w, `{"dubbing_id":"dub_1","expected_duration_sec":60}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/dubbing/dub_1":
			_, _ = io.WriteString(w, `{"dubbing_id":"dub_1","status":"dubbed","media_metadata":{"duration":60}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/flows/video":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"sync-lipsync-v3"`) {
				t.Errorf("model not forwarded: %s", body)
			}
			_, _ = io.WriteString(w, `{"id":"gen_1","status":"pending"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/flows/video/gen_1":
			_, _ = io.WriteString(w, `{"id":"gen_1","status":"completed","content_url":"https://storage.example/out.mp4","content_mime_type":"video/mp4"}`)
		default:
			http.NotFound(w, r)
		}
	}, nil)

	resp, err := http.Post(ts.URL+"/elevenlabs/v1/dubbing", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("X-Nabu-Cost-USD") != "0.500000" {
		t.Fatalf("dub create cost %q", resp.Header.Get("X-Nabu-Cost-USD"))
	}
	resp, err = http.Get(ts.URL + "/elevenlabs/v1/dubbing/dub_1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("X-Nabu-Cost-USD") != "0.000000" {
		t.Fatalf("dub poll was billed again: %s", resp.Header.Get("X-Nabu-Cost-USD"))
	}

	resp, err = http.Post(ts.URL+"/elevenlabs/v1/flows/video", "application/json",
		strings.NewReader(`{"model_id":"sync-lipsync-v3","duration_secs":10}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), `"content_url"`) && !strings.Contains(string(raw), "pending") {
		t.Fatalf("create body %s", raw)
	}
	// 10 seconds at $7.20 / minute.
	if resp.Header.Get("X-Nabu-Cost-USD") != "1.200000" {
		t.Fatalf("lipsync cost %q body %s", resp.Header.Get("X-Nabu-Cost-USD"), raw)
	}
	resp, err = http.Get(ts.URL + "/elevenlabs/v1/flows/video/gen_1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(got), "storage.example") {
		t.Fatalf("result body %s", got)
	}
	if resp.Header.Get("X-Nabu-Cost-USD") != "0.000000" {
		t.Fatalf("lipsync poll billed again: %s", resp.Header.Get("X-Nabu-Cost-USD"))
	}
}

func TestElevenLabsProxyTranscriptionMetersTheVendorDuration(t *testing.T) {
	ts := elevenGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/speech-to-text" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("xi-api-key") != "gateway-secret" {
			t.Errorf("key %q", r.Header.Get("xi-api-key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"سلام","language_code":"fa","audio_duration_secs":60}`)
	}, nil)
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	part, _ := form.CreateFormFile("file", "note.mp3")
	_, _ = part.Write([]byte("ID3"))
	_ = form.WriteField("model_id", "scribe_v2")
	_ = form.WriteField("language_code", "fa")
	_ = form.Close()
	resp, err := http.Post(ts.URL+"/elevenlabs/v1/speech-to-text", form.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"سلام"`) {
		t.Fatalf("body %s", body)
	}
	if resp.Header.Get("X-Nabu-Cost-USD") != "0.003667" {
		t.Fatalf("cost %q", resp.Header.Get("X-Nabu-Cost-USD"))
	}
}

func TestElevenLabsProxyReportsMissingKey(t *testing.T) {
	r := router.New(map[string]provider.Adapter{}, nil, nil, nil, nil, nil, nil, discardLogger())
	srv := New(r, policy.New(nil, nil), usage.New(nil), nil, discardLogger())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/elevenlabs/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ELEVENLABS_API_KEY") {
		t.Fatalf("body %s", body)
	}
}

func TestElevenCredentialHonoursOwnKeyAndGatewayDenial(t *testing.T) {
	proxy := provider.NewElevenLabsAdapter("elevenlabs", "https://api.elevenlabs.io/v1", "gateway-secret").Proxy()
	s := &Server{}

	req, _ := http.NewRequest(http.MethodGet, "http://example/elevenlabs/v1/voices", nil)
	ctx := context.WithValue(req.Context(), router.CallerKeysCtxKey{}, router.CallerKeys{
		Keys: map[string][]string{"elevenlabs": {"caller-key"}},
		Mode: router.ModeOwnFirst,
	})
	ctx = context.WithValue(ctx, router.GatewayDeniedCtxKey{}, map[string]bool{"elevenlabs": true})
	req = req.WithContext(ctx)
	key, byCaller, code, _ := s.elevenCredential(req, proxy)
	if code != 0 || !byCaller || key != "caller-key" {
		t.Fatalf("own-first denied gateway: key=%q caller=%v code=%d", key, byCaller, code)
	}

	ctx = context.WithValue(req.Context(), router.CallerKeysCtxKey{}, router.CallerKeys{Mode: router.ModeGlobal})
	ctx = context.WithValue(ctx, router.GatewayDeniedCtxKey{}, map[string]bool{"elevenlabs": true})
	req = req.WithContext(ctx)
	_, _, code, msg := s.elevenCredential(req, proxy)
	if code != http.StatusForbidden || !strings.Contains(msg, "approval") {
		t.Fatalf("global denied: %d %s", code, msg)
	}

	ctx = context.WithValue(context.Background(), router.CallerKeysCtxKey{}, router.CallerKeys{Mode: router.ModeGlobal})
	req = req.WithContext(ctx)
	key, byCaller, code, _ = s.elevenCredential(req, proxy)
	if code != 0 || byCaller || key != "gateway-secret" {
		t.Fatalf("global: key=%q caller=%v code=%d", key, byCaller, code)
	}
}

func TestElevenProviderAllowList(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), router.AllowedProvidersCtxKey{}, []string{"openai"})
	req = req.WithContext(ctx)
	if elevenProviderAllowed(req) {
		t.Fatal("openai-only key was allowed to spend elevenlabs")
	}
	ctx = context.WithValue(req.Context(), router.AllowedProvidersCtxKey{}, []string{"elevenlabs"})
	req = req.WithContext(ctx)
	if !elevenProviderAllowed(req) {
		t.Fatal("elevenlabs key was refused")
	}
}
