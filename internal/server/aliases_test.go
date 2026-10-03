package server

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"nabugate/internal/adminstore"
	"nabugate/internal/agent"
	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// studio answers every capability the console and the meters care about.
// Its embeddings report no usage at all, as Gemini's do.
type studio struct{ name string }

func (s studio) Name() string { return s.name }
func (s studio) Chat(context.Context, provider.ChatRequest) (provider.ChatResponse, error) {
	return provider.ChatResponse{Content: "from " + s.name, Usage: provider.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}}, nil
}
func (s studio) Speech(context.Context, provider.SpeechRequest) (provider.SpeechResponse, error) {
	return provider.SpeechResponse{Audio: []byte("a"), ContentType: "audio/mpeg"}, nil
}
func (s studio) Image(context.Context, provider.ImageRequest) (provider.ImageResponse, error) {
	return provider.ImageResponse{Images: []string{"aGk=", "aGk="}}, nil
}
func (s studio) Embed(context.Context, provider.EmbeddingRequest) (provider.EmbeddingResponse, error) {
	return provider.EmbeddingResponse{Embeddings: [][]float64{{0.1}}}, nil
}

type broken struct{ studio }

func (broken) Chat(context.Context, provider.ChatRequest) (provider.ChatResponse, error) {
	return provider.ChatResponse{}, errors.New("down")
}

func studioServer(t *testing.T, statePath string) (*httptest.Server, *adminstore.Store, string, string) {
	t.Helper()
	store, err := adminstore.Open(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if store.NeedsSetup() {
		if err := store.CreateAdmin("root", "correct-horse-battery"); err != nil {
			t.Fatal(err)
		}
		if err := store.SignupUser("guest@example.com", "a-real-password"); err != nil {
			t.Fatal(err)
		}
	}
	admin, _, err := store.Authenticate("root", "correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := store.AuthenticateUser("guest@example.com", "a-real-password")
	if err != nil {
		t.Fatal(err)
	}

	adapters := map[string]provider.Adapter{
		"openai": studio{name: "openai"},
		"gemini": studio{name: "gemini"},
	}
	models := map[string]config.ModelRoute{
		"nabu-fast": {
			Primary:  config.Target{Provider: "openai", Model: "gpt-4o-mini"},
			Fallback: []config.Target{{Provider: "gemini", Model: "gemini-2.5-flash"}},
		},
	}
	audio := map[string]config.ModelRoute{
		"nabu-voice": {Primary: config.Target{Provider: "openai", Model: "gpt-4o-mini-tts"}, Voice: "marin"},
	}
	images := map[string]config.ModelRoute{"nabu-image": {Primary: config.Target{Provider: "openai", Model: "gpt-image-1"}}}
	embeddings := map[string]config.ModelRoute{"nabu-embed": {Primary: config.Target{Provider: "gemini", Model: "gemini-embedding-001"}}}
	r := router.New(adapters, models, images, audio, embeddings, nil, nil, discardLogger())

	prices := map[string]usage.Price{
		"gpt-4o-mini":          {Input: 0.15, Output: 0.6},
		"gemini-2.5-flash":     {Input: 0.3, Output: 2.5},
		"gpt-4o-mini-tts":      {PerMillionChars: 15},
		"gpt-image-1":          {PerImage: 0.04},
		"gemini-embedding-001": {Input: 0.15},
	}
	srv := New(r, policy.New(nil, nil), usage.New(prices), agent.NewRegistry(), discardLogger())
	srv.SetAdminStore(store)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, store, admin, user
}

func chargedOn(t *testing.T, resp *http.Response) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(resp.Header.Get(costHeader), 64)
	if err != nil {
		t.Fatalf("no cost header on %s: %q", resp.Request.URL.Path, resp.Header.Get(costHeader))
	}
	return v
}

func post(t *testing.T, ts *httptest.Server, path, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(ts.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s answered %d", path, resp.StatusCode)
	}
	return resp
}

// Speech, images and embeddings were served without ever reaching a meter: no
// cost, no balance moving, nothing in the console. Each is now billed in the
// unit its vendor charges in.
func TestEveryCapabilityIsMeteredInItsOwnUnit(t *testing.T) {
	ts, store, _, _ := studioServer(t, t.TempDir()+"/state.json")

	// Twelve characters, counted as characters rather than as bytes of Persian.
	speech := post(t, ts, "/v1/audio/speech", `{"model":"nabu-voice","input":"سلام، چطوری؟"}`)
	if got, want := chargedOn(t, speech), 12.0/1e6*15; math.Abs(got-want) > 1e-6 {
		t.Errorf("speech charged %v, want %v", got, want)
	}

	images := post(t, ts, "/v1/images/generations", `{"model":"nabu-image","prompt":"a lemon"}`)
	if got := chargedOn(t, images); math.Abs(got-0.08) > 1e-6 {
		t.Errorf("two images charged %v, want 0.08", got)
	}

	// The upstream reported no tokens; forty thousand bytes are counted as ten
	// thousand.
	embed := post(t, ts, "/v1/embeddings", `{"model":"nabu-embed","input":"`+strings.Repeat("a", 40_000)+`"}`)
	if got, want := chargedOn(t, embed), 10_000.0/1e6*0.15; math.Abs(got-want) > 1e-6 {
		t.Errorf("embedding charged %v, want %v", got, want)
	}

	_, _, byProvider := store.Usage()
	if c := byProvider["openai"]; c.Requests != 2 || c.ProviderCostUSD <= 0 || math.Abs(c.ProviderCostUSD-c.CostUSD) > 1e-12 {
		t.Errorf("openai counters = %+v; want two requests at list price, with no plan markup", c)
	}
}

// The console moves an alias to another configured model and picks its voice;
// the next request follows, nobody else may, and the choice outlives a restart.
func TestTheConsoleChoosesAnAliasModelAndVoice(t *testing.T) {
	path := t.TempDir() + "/state.json"
	ts, _, admin, user := studioServer(t, path)

	if resp, _ := consoleDo(t, ts, user, http.MethodPut, "/api/aliases/nabu-fast", `{"primary":"gemini/gemini-2.5-flash"}`); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a non-admin changed routing: %d", resp.StatusCode)
	}
	resp, list := consoleDo(t, ts, user, http.MethodGet, "/api/aliases", "")
	if resp.StatusCode != http.StatusOK || list["can_edit"] != false {
		t.Fatalf("a signed-in user should read the routing and its prices: %d %v", resp.StatusCode, list["can_edit"])
	}

	for _, bad := range []struct{ alias, body string }{
		{"nabu-fast", `{"primary":"anthropic/claude"}`},
		{"nabu-fast", `{"voice":"marin"}`},
		{"nabu-voice", `{"voice":"` + strings.Repeat("x", 65) + `"}`},
	} {
		if resp, _ := consoleDo(t, ts, admin, http.MethodPut, "/api/aliases/"+bad.alias, bad.body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s %s answered %d, want 400", bad.alias, bad.body, resp.StatusCode)
		}
	}

	resp, view := consoleDo(t, ts, admin, http.MethodPut, "/api/aliases/nabu-fast", `{"primary":"gemini/gemini-2.5-flash"}`)
	if resp.StatusCode != http.StatusOK || view["primary"] != "gemini/gemini-2.5-flash" || view["config_primary"] != "openai/gpt-4o-mini" {
		t.Fatalf("saving answered %d %v", resp.StatusCode, view)
	}
	if resp, _ := consoleDo(t, ts, admin, http.MethodPut, "/api/aliases/nabu-voice", `{"voice":"cedar"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("saving a voice answered %d", resp.StatusCode)
	}

	chat := post(t, ts, "/v1/chat/completions", `{"model":"nabu-fast","messages":[{"role":"user","content":"hi"}]}`)
	if got := chat.Header.Get("X-Nabu-Provider"); got != "gemini" {
		t.Fatalf("the next request went to %q, want gemini", got)
	}

	// A restart reads the choice back from the state file.
	ts.Close()
	ts2, _, _, _ := studioServer(t, path)
	chat = post(t, ts2, "/v1/chat/completions", `{"model":"nabu-fast","messages":[{"role":"user","content":"hi"}]}`)
	if got := chat.Header.Get("X-Nabu-Provider"); got != "gemini" {
		t.Fatalf("after a restart the request went to %q, want gemini", got)
	}

	// Empty puts the alias back to the config's model.
	if resp, _ := consoleDo(t, ts2, admin, http.MethodPut, "/api/aliases/nabu-fast", `{"primary":""}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("resetting answered %d", resp.StatusCode)
	}
	chat = post(t, ts2, "/v1/chat/completions", `{"model":"nabu-fast","messages":[{"role":"user","content":"hi"}]}`)
	if got := chat.Header.Get("X-Nabu-Provider"); got != "openai" {
		t.Fatalf("after a reset the request went to %q, want openai", got)
	}
}
