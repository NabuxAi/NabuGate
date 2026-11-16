package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCreateLiveSessionForwardsSessionToolsUnchanged(t *testing.T) {
	tools := `[{"type":"function","name":"search_products","description":"Find products","parameters":{"type":"object","properties":{"q":{"type":"string","description":"query"}},"required":["q"],"additionalProperties":false}}]`
	body := `{"model":"nabu-live","session":{"type":"realtime","instructions":"shop","tool_choice":"auto","tools":` + tools + `},"transport":{"type":"webrtc","sdp":"v=0 offer"}}`
	var sessionField string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/realtime/calls" {
			t.Errorf("path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return
		}
		sessionField = r.FormValue("session")
		w.Header().Set("Location", "/v1/realtime/calls/call_tools")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("v=0\r\n"))
	}))
	defer srv.Close()

	_, err := NewOpenAIAdapter("openai", srv.URL, "test-key", nil).CreateLiveSession(context.Background(),
		LiveSessionRequest{Model: "gpt-realtime-2.1", Body: json.RawMessage(body)})
	if err != nil {
		t.Fatal(err)
	}
	var session map[string]any
	if err := json.Unmarshal([]byte(sessionField), &session); err != nil {
		t.Fatalf("session field: %s", sessionField)
	}
	if session["model"] != "gpt-realtime-2.1" {
		t.Fatalf("model = %v", session["model"])
	}
	got, _ := json.Marshal(session["tools"])
	var want, decoded any
	if err := json.Unmarshal([]byte(tools), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatal(err)
	}
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(decoded)
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("tools changed\nwant %s\ngot  %s", wantJSON, gotJSON)
	}
	if session["tool_choice"] != "auto" || session["instructions"] != "shop" {
		t.Fatalf("session fields dropped: %s", sessionField)
	}
}

func TestRewriteLiveModelKeepsSessionTools(t *testing.T) {
	tools := `[{"type":"function","name":"add_to_cart","description":"Add","parameters":{"type":"object","properties":{"sku":{"type":"string"}}}}]`
	out, err := rewriteLiveModel(json.RawMessage(
		`{"model":"nabu-live","session":{"model":"nabu-live","tools":`+tools+`,"tool_choice":"auto"}}`,
	), "gpt-realtime")
	if err != nil {
		t.Fatal(err)
	}
	var top struct {
		Session struct {
			Model      string          `json:"model"`
			ToolChoice string          `json:"tool_choice"`
			Tools      json.RawMessage `json:"tools"`
		} `json:"session"`
	}
	if err := json.Unmarshal(out, &top); err != nil {
		t.Fatal(err)
	}
	if top.Session.Model != "gpt-realtime" || top.Session.ToolChoice != "auto" {
		t.Fatalf("session = %+v", top.Session)
	}
	if string(top.Session.Tools) != tools {
		t.Fatalf("tools = %s", top.Session.Tools)
	}
}

func TestRewriteLiveModelReplacesTheAliasWithTheUpstreamModel(t *testing.T) {
	out, err := rewriteLiveModel(json.RawMessage(
		`{"model":"nabu-live","session":{"model":"nabu-live","instructions":"be brief"},"transport":{"type":"webrtc","sdp":"v=0 offer"}}`,
	), "gpt-live-1")
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(out, &top); err != nil {
		t.Fatal(err)
	}
	if _, leaked := top["model"]; leaked {
		t.Fatalf("the gateway alias reached the vendor: %s", out)
	}
	var session map[string]any
	if err := json.Unmarshal(top["session"], &session); err != nil {
		t.Fatal(err)
	}
	if session["model"] != "gpt-live-1" || session["instructions"] != "be brief" {
		t.Fatalf("session = %v", session)
	}
	if !strings.Contains(string(top["transport"]), `"sdp":"v=0 offer"`) {
		t.Fatalf("offer not forwarded untouched: %s", top["transport"])
	}
}

func TestRewriteLiveModelOnEmptyAndMalformedBodies(t *testing.T) {
	out, err := rewriteLiveModel(nil, "gpt-live-1")
	if err != nil || string(out) != `{"session":{"model":"gpt-live-1"}}` {
		t.Fatalf("empty body: %s, %v", out, err)
	}
	if _, err := rewriteLiveModel(json.RawMessage(`[1,2]`), "m"); err == nil {
		t.Fatal("a non-object body was accepted")
	}
	if _, err := rewriteLiveModel(json.RawMessage(`{"session":"not an object"}`), "m"); err == nil {
		t.Fatal("a non-object session was accepted")
	}
}

func TestCreateLiveSessionReturnsTheVendorsAnswer(t *testing.T) {
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"session":{"id":"live_9"},"transport":{"type":"webrtc","sdp":"v=0 answer"}}`))
	}))
	defer srv.Close()

	resp, err := NewOpenAIAdapter("openai", srv.URL, "k", nil).CreateLiveSession(context.Background(),
		LiveSessionRequest{Model: "gpt-live-1", Body: json.RawMessage(`{"transport":{"sdp":"o"}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "live_9" || !strings.Contains(string(resp.Body), "v=0 answer") {
		t.Fatalf("answer not passed through: %+v", resp)
	}
	if auth != "Bearer k" || (path != "/realtime/calls" && path != "/realtime/sessions") {
		t.Fatalf("auth %q path %q", auth, path)
	}
}

func TestCreateLiveSessionOpenAIGaWithLocationHeader(t *testing.T) {
	var auth, path, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		ctype = r.Header.Get("Content-Type")
		w.Header().Set("Location", "/v1/realtime/calls/call_ga_123")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("v=0\r\no=- 999 2 IN IP4 127.0.0.1\r\ns=-\r\n"))
	}))
	defer srv.Close()

	resp, err := NewOpenAIAdapter("openai", srv.URL, "k", nil).CreateLiveSession(context.Background(),
		LiveSessionRequest{Model: "gpt-4o-realtime-preview", Body: json.RawMessage(`{"transport":{"type":"webrtc","sdp":"v=0\r\no=offer"}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "call_ga_123" {
		t.Fatalf("expected ID call_ga_123, got %q", resp.ID)
	}
	if auth != "Bearer k" || path != "/realtime/calls" {
		t.Fatalf("auth %q path %q", auth, path)
	}
	if !strings.HasPrefix(ctype, "multipart/form-data;") {
		t.Fatalf("expected multipart content-type, got %q", ctype)
	}
	var parsed struct {
		ID      string `json:"id"`
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
		Transport struct {
			SDP string `json:"sdp"`
		} `json:"transport"`
	}
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		t.Fatalf("invalid json body: %v, raw: %s", err, resp.Body)
	}
	if parsed.Session.ID != "call_ga_123" || !strings.Contains(parsed.Transport.SDP, "IN IP4 127.0.0.1") {
		t.Fatalf("unexpected normalized body: %+v", parsed)
	}
}

func TestCreateLiveSessionIsAttemptedExactlyOnce(t *testing.T) {
	// Every other endpoint retries a 5xx. This one must not: the vendor may
	// already have opened the session whose answer was lost, and a replayed
	// offer opens a second one nothing ever closes.
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream hiccup"}}`))
	}))
	defer srv.Close()

	_, err := NewOpenAIAdapter("openai", srv.URL, "k", nil).CreateLiveSession(context.Background(),
		LiveSessionRequest{Model: "gpt-live-1", Body: json.RawMessage(`{}`)})
	var refusal *LiveUpstreamError
	if !errors.As(err, &refusal) || refusal.Status != http.StatusBadGateway {
		t.Fatalf("err = %v", err)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("session creation attempted %d times", n)
	}
}

func TestCreateLiveSessionSurfacesTheVendorsMessageWithoutSecrets(t *testing.T) {
	cases := []struct {
		name, body, want, mustNot string
		status                    int
	}{
		{"structured", `{"error":{"message":"Invalid SDP offer: missing fingerprint","type":"invalid_request_error"}}`, "Invalid SDP offer: missing fingerprint", "", http.StatusBadRequest},
		{"echoed key", `{"error":{"message":"Incorrect API key provided: sk-proj-abcdefghijklmnop."}}`, "[redacted]", "abcdefghijklmnop", http.StatusUnauthorized},
		{"html page", "<html>\n<body>bad gateway</body>\n</html>", "<html> <body>bad gateway</body> </html>", "\n", http.StatusBadGateway},
		{"empty", ``, "(empty response)", "", http.StatusServiceUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()

			_, err := NewOpenAIAdapter("openai", srv.URL, "k", nil).CreateLiveSession(context.Background(),
				LiveSessionRequest{Model: "gpt-live-1", Body: json.RawMessage(`{}`)})
			var refusal *LiveUpstreamError
			if !errors.As(err, &refusal) {
				t.Fatalf("err = %v", err)
			}
			if refusal.Status != c.status || !strings.Contains(refusal.Message, c.want) {
				t.Fatalf("refusal = %+v", refusal)
			}
			if c.mustNot != "" && strings.Contains(refusal.Message, c.mustNot) {
				t.Fatalf("message leaked %q: %q", c.mustNot, refusal.Message)
			}
		})
	}
}

func TestCreateLiveSessionMintsAGAClientSecretAndKeepsTools(t *testing.T) {
	tools := `[{"type":"function","name":"search_products","description":"Find products","parameters":{"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"additionalProperties":false}}]`
	body := `{"model":"nabu-live","session":{"instructions":"shop","voice":"marin","tool_choice":"auto","tools":` + tools + `}}`
	var got []byte
	var path, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		if r.URL.Path == "/realtime/sessions" {
			t.Errorf("the retired sessions endpoint was called")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"Invalid URL (POST /v1/realtime/sessions)"}}`))
			return
		}
		got, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":"ek_test_secret","expires_at":1893456000,"session":{"id":"sess_ga","type":"realtime","model":"gpt-realtime-2.1"}}`))
	}))
	defer srv.Close()

	resp, err := NewOpenAIAdapter("openai", srv.URL, "k", nil).CreateLiveSession(context.Background(),
		LiveSessionRequest{Model: "gpt-realtime-2.1", Body: json.RawMessage(body)})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/realtime/client_secrets" || auth != "Bearer k" {
		t.Fatalf("auth %q path %q", auth, path)
	}
	var sent struct {
		Session struct {
			Type         string          `json:"type"`
			Model        string          `json:"model"`
			Instructions string          `json:"instructions"`
			ToolChoice   string          `json:"tool_choice"`
			Voice        string          `json:"voice"`
			Tools        json.RawMessage `json:"tools"`
			Audio        struct {
				Output struct {
					Voice string `json:"voice"`
				} `json:"output"`
			} `json:"audio"`
		} `json:"session"`
	}
	if err := json.Unmarshal(got, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Session.Type != "realtime" || sent.Session.Model != "gpt-realtime-2.1" {
		t.Fatalf("session identity = %+v", sent.Session)
	}
	if sent.Session.Instructions != "shop" || sent.Session.ToolChoice != "auto" || sent.Session.Voice != "" {
		t.Fatalf("session fields = %+v", sent.Session)
	}
	if sent.Session.Audio.Output.Voice != "marin" {
		t.Fatalf("voice = %q", sent.Session.Audio.Output.Voice)
	}
	var want, decoded any
	if err := json.Unmarshal([]byte(tools), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(sent.Session.Tools, &decoded); err != nil {
		t.Fatalf("tools = %s", sent.Session.Tools)
	}
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(decoded)
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("tools changed\nwant %s\ngot  %s", wantJSON, gotJSON)
	}
	if resp.ID != "sess_ga" {
		t.Fatalf("id = %q", resp.ID)
	}
	var answer struct {
		Token        string `json:"token"`
		Value        string `json:"value"`
		URL          string `json:"url"`
		ClientSecret struct {
			Value string `json:"value"`
		} `json:"client_secret"`
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if err := json.Unmarshal(resp.Body, &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Token != "ek_test_secret" || answer.Value != "ek_test_secret" || answer.ClientSecret.Value != "ek_test_secret" {
		t.Fatalf("ephemeral key not in the client shape: %s", resp.Body)
	}
	if answer.Session.ID != "sess_ga" || !strings.HasSuffix(answer.URL, "/realtime/calls") {
		t.Fatalf("answer = %+v", answer)
	}
}

func TestCreateLiveSessionIgnoresARetiredSessions404WhenTheClientSecretWorks(t *testing.T) {
	// OpenAI's GA API answers 404 "Invalid URL" for POST /realtime/sessions.
	// A JSON create with no SDP offer must still succeed via client_secrets.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/realtime/sessions" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"Invalid URL (POST /v1/realtime/sessions)"}}`))
			return
		}
		if r.URL.Path != "/realtime/client_secrets" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"value":"ek_ok","expires_at":1893456000,"session":{"id":"sess_ok"}}`))
	}))
	defer srv.Close()

	resp, err := NewOpenAIAdapter("openai", srv.URL, "k", nil).CreateLiveSession(context.Background(),
		LiveSessionRequest{Model: "gpt-realtime-2.1", Body: json.RawMessage(`{"model":"nabu-live"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "sess_ok" || !strings.Contains(string(resp.Body), "ek_ok") || !strings.Contains(string(resp.Body), "/realtime/calls") {
		t.Fatalf("answer = %s err=%v", resp.Body, err)
	}
}

func TestCreateLiveSessionFallsBackToLegacySessionsWhenClientSecrets404(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/realtime/client_secrets" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"no client secrets here"}}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"session":{"id":"live_legacy"},"client_secret":{"value":"ek_legacy"}}`))
	}))
	defer srv.Close()

	resp, err := NewOpenAIAdapter("openai", srv.URL, "k", nil).CreateLiveSession(context.Background(),
		LiveSessionRequest{Model: "gpt-live-1", Body: json.RawMessage(`{"model":"nabu-live"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "live_legacy" || !strings.Contains(string(resp.Body), "ek_legacy") {
		t.Fatalf("legacy answer = %s", resp.Body)
	}
	if len(paths) != 2 || paths[0] != "/realtime/client_secrets" || paths[1] != "/realtime/sessions" {
		t.Fatalf("paths = %v", paths)
	}
}

func TestCreateLiveSessionReportsTheGaRefusalNotTheRetiredEndpoints(t *testing.T) {
	// OpenAI answers 404 for a model it does not serve, and the retired
	// /realtime/sessions answers 404 "Invalid URL" for everything. Falling
	// through to the second and reporting its message hid the real reason
	// for a fortnight: the alias was pointed at a model that no longer exists.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		if r.URL.Path == "/realtime/calls" {
			_, _ = w.Write([]byte(`{"error":{"message":"The model 'gpt-4o-realtime-preview' does not exist","code":"model_not_found"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid URL (POST /v1/realtime/sessions)"}}`))
	}))
	defer srv.Close()

	_, err := NewOpenAIAdapter("openai", srv.URL, "k", nil).CreateLiveSession(context.Background(),
		LiveSessionRequest{Model: "gpt-4o-realtime-preview", Body: json.RawMessage(`{"transport":{"type":"webrtc","sdp":"v=0 offer"}}`)})
	var refusal *LiveUpstreamError
	if !errors.As(err, &refusal) || refusal.Status != http.StatusNotFound {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(refusal.Message, "does not exist") || strings.Contains(refusal.Message, "Invalid URL") {
		t.Fatalf("the retired endpoint's message was reported instead of the vendor's: %q", refusal.Message)
	}
}
