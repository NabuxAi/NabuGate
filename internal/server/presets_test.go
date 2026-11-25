package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/preset"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

func testPresets(t *testing.T) *preset.Registry {
	t.Helper()
	reg := preset.NewRegistry()
	zero := 0.0
	if err := reg.AddSkill(preset.Skill{Name: "json-extract", System: "Answer in JSON.", ResponseFormat: "json_object", Temperature: &zero}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddSkill(preset.Skill{Name: "concise", System: "Be brief."}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddTemplate(preset.Template{
		Name: "translate", Model: "nabu-fast", System: "You translate.",
		Prompt: "Translate into {{target}}:\n{{text}}",
		Variables: []preset.Variable{
			{Name: "text", Required: true},
			{Name: "target", Default: "Persian", Options: []string{"Persian", "English"}},
		},
		Skills: []string{"concise"},
	}); err != nil {
		t.Fatal(err)
	}
	return reg
}

func newPresetServer(t *testing.T, upstreamURL string) *httptest.Server {
	t.Helper()
	adapters := map[string]provider.Adapter{"parspack": provider.NewOpenAIAdapter("parspack", upstreamURL, "k", nil)}
	models := map[string]config.ModelRoute{"nabu-fast": {Primary: config.Target{Provider: "parspack", Model: "openai/gpt-5.5"}}}
	r := router.New(adapters, models, nil, nil, nil, nil, map[string][]string{"parspack": nil}, discardLogger())
	srv := New(r, policy.New(nil, nil), usage.New(nil), nil, discardLogger()).WithPresets(testPresets(t))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestChatSkillsInjectAndStripField(t *testing.T) {
	cap, h := capturingUpstream(t)
	up := newHTTPServer(t, h)
	ts := newPresetServer(t, up.URL)

	body := `{"model":"nabu-fast","nabu_skills":["json-extract"],"messages":[{"role":"system","content":"Caller rules."},{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set(skillsHeader, "concise")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get(skillsHeader); got != "json-extract,concise" {
		t.Errorf("%s = %q", skillsHeader, got)
	}
	got := cap.get()
	if _, leaked := got["nabu_skills"]; leaked {
		t.Error("nabu_skills was forwarded upstream")
	}
	if role, content := nthMessage(t, got, 0); role != "system" || content != "Caller rules." {
		t.Errorf("message[0] = %q/%q, want the caller's system prompt first", role, content)
	}
	if role, content := nthMessage(t, got, 1); role != "system" || content != "Answer in JSON.\n\nBe brief." {
		t.Errorf("message[1] = %q/%q, want the skills' system text", role, content)
	}
	if role, _ := nthMessage(t, got, 2); role != "user" {
		t.Errorf("message[2] role = %q, want user", role)
	}
	if string(got["response_format"]) != `{"type":"json_object"}` {
		t.Errorf("response_format = %s", got["response_format"])
	}
	if string(got["temperature"]) != "0" {
		t.Errorf("temperature = %s, want skill default 0", got["temperature"])
	}
}

func TestChatSkillCallerParamsWin(t *testing.T) {
	cap, h := capturingUpstream(t)
	up := newHTTPServer(t, h)
	ts := newPresetServer(t, up.URL)
	body := `{"model":"nabu-fast","temperature":0.7,"response_format":{"type":"text"},"nabu_skills":"json-extract","messages":[{"role":"user","content":"hi"}]}`
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	got := cap.get()
	if string(got["temperature"]) != "0.7" || string(got["response_format"]) != `{"type":"text"}` {
		t.Errorf("caller params overridden: temperature=%s response_format=%s", got["temperature"], got["response_format"])
	}
}

func TestChatUnknownSkillIs400(t *testing.T) {
	_, h := capturingUpstream(t)
	up := newHTTPServer(t, h)
	ts := newPresetServer(t, up.URL)
	body := `{"model":"nabu-fast","nabu_skills":["nope"],"messages":[{"role":"user","content":"hi"}]}`
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestTemplateRun(t *testing.T) {
	cap, h := capturingUpstream(t)
	up := newHTTPServer(t, h)
	ts := newPresetServer(t, up.URL)

	resp, err := http.Post(ts.URL+"/v1/templates/translate/run", "application/json",
		strings.NewReader(`{"variables":{"text":"hello","target":"English"}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Nabu-Template") != "translate" || resp.Header.Get(skillsHeader) != "concise" {
		t.Errorf("headers: template=%q skills=%q", resp.Header.Get("X-Nabu-Template"), resp.Header.Get(skillsHeader))
	}
	got := cap.get()
	if role, content := nthMessage(t, got, 0); role != "system" || content != "You translate." {
		t.Errorf("message[0] = %q/%q", role, content)
	}
	if _, content := nthMessage(t, got, 2); content != "Translate into English:\nhello" {
		t.Errorf("user prompt = %q", content)
	}
	var out struct {
		Object string `json:"object"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Object != "chat.completion" {
		t.Errorf("object = %q, want chat.completion", out.Object)
	}
}

func TestTemplateRunValidation(t *testing.T) {
	_, h := capturingUpstream(t)
	up := newHTTPServer(t, h)
	ts := newPresetServer(t, up.URL)
	for name, body := range map[string]string{
		"missing required": `{"variables":{}}`,
		"bad option":       `{"variables":{"text":"x","target":"Klingon"}}`,
		"unknown variable": `{"variables":{"text":"x","foo":"y"}}`,
	} {
		resp, err := http.Post(ts.URL+"/v1/templates/translate/run", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, resp.StatusCode)
		}
	}
	resp, _ := http.Post(ts.URL+"/v1/templates/none/run", "application/json", strings.NewReader(`{}`))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown template: status = %d, want 404", resp.StatusCode)
	}
}

func TestPresetLists(t *testing.T) {
	_, h := capturingUpstream(t)
	up := newHTTPServer(t, h)
	ts := newPresetServer(t, up.URL)
	for path, want := range map[string]int{"/v1/skills": 2, "/v1/templates": 1} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			Data []json.RawMessage `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if len(out.Data) != want {
			t.Errorf("%s: %d items, want %d", path, len(out.Data), want)
		}
	}
}
