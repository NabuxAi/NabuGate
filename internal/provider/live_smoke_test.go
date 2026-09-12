package provider

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// smokeOffer is a synthetic WebRTC offer: one Opus audio track and the data
// channel, the shape a browser sends. It never connects — nothing answers its
// ICE candidates — which is the point: the vendor answers the signalling, and
// the session ends when ICE gives up.
const smokeOffer = "v=0\r\n" +
	"o=- 4611731400430051336 2 IN IP4 127.0.0.1\r\n" +
	"s=-\r\n" +
	"t=0 0\r\n" +
	"a=group:BUNDLE 0 1\r\n" +
	"a=msid-semantic: WMS\r\n" +
	"m=audio 9 UDP/TLS/RTP/SAVPF 111\r\n" +
	"c=IN IP4 0.0.0.0\r\n" +
	"a=rtcp:9 IN IP4 0.0.0.0\r\n" +
	"a=ice-ufrag:nbgt\r\n" +
	"a=ice-pwd:nabugatesmoketestpassword00\r\n" +
	"a=ice-options:trickle\r\n" +
	"a=fingerprint:sha-256 4A:AD:B9:B1:3F:82:18:3B:54:02:12:DF:3E:5D:49:6B:19:E5:7C:AB:3E:A3:A8:7B:3F:94:1D:52:D6:1F:73:18\r\n" +
	"a=setup:actpass\r\n" +
	"a=mid:0\r\n" +
	"a=sendrecv\r\n" +
	"a=rtcp-mux\r\n" +
	"a=rtpmap:111 opus/48000/2\r\n" +
	"a=fmtp:111 minptime=10;useinbandfec=1\r\n" +
	"m=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\n" +
	"c=IN IP4 0.0.0.0\r\n" +
	"a=ice-ufrag:nbgt\r\n" +
	"a=ice-pwd:nabugatesmoketestpassword00\r\n" +
	"a=ice-options:trickle\r\n" +
	"a=fingerprint:sha-256 4A:AD:B9:B1:3F:82:18:3B:54:02:12:DF:3E:5D:49:6B:19:E5:7C:AB:3E:A3:A8:7B:3F:94:1D:52:D6:1F:73:18\r\n" +
	"a=setup:actpass\r\n" +
	"a=mid:1\r\n" +
	"a=sctp-port:5000\r\n" +
	"a=max-message-size:262144\r\n"

// TestLiveSmokeAgainstTheRealVendor signals one real GPT-Live session. Opt-in
// only — it spends a few seconds of a real session — and never part of a plain
// `go test ./...`:
//
//	NABUGATE_LIVE_SMOKE=1 OPENAI_API_KEY=sk-… go test ./internal/provider -run TestLiveSmoke -v
//
// NABUGATE_LIVE_SMOKE_OFFER points at a captured browser offer if the vendor
// rejects the synthetic one; NABUGATE_LIVE_SMOKE_BASE_URL and
// NABUGATE_LIVE_SMOKE_MODEL override the vendor endpoint and model.
func TestLiveSmokeAgainstTheRealVendor(t *testing.T) {
	if os.Getenv("NABUGATE_LIVE_SMOKE") != "1" {
		t.Skip("set NABUGATE_LIVE_SMOKE=1 and OPENAI_API_KEY to signal one real GPT-Live session")
	}
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	offer := smokeOffer
	if path := os.Getenv("NABUGATE_LIVE_SMOKE_OFFER"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		offer = string(raw)
	}
	base := os.Getenv("NABUGATE_LIVE_SMOKE_BASE_URL")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	model := os.Getenv("NABUGATE_LIVE_SMOKE_MODEL")
	if model == "" {
		model = "gpt-live-1"
	}

	// Client delegation: no backend model runs unless somebody answers, so the
	// check costs the session's seconds and nothing else.
	body, _ := json.Marshal(map[string]any{
		"session":   map[string]any{"instructions": "Say one short greeting.", "delegation": map[string]any{"type": "client"}},
		"transport": map[string]any{"type": "webrtc", "sdp": offer},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := NewOpenAIAdapter("openai", base, key, nil).CreateLiveSession(ctx, LiveSessionRequest{Model: model, Body: body})
	if err != nil {
		t.Fatalf("the vendor refused the session: %v", err)
	}
	var answer struct {
		Transport struct {
			SDP string `json:"sdp"`
		} `json:"transport"`
	}
	_ = json.Unmarshal(resp.Body, &answer)
	if resp.ID == "" || !strings.HasPrefix(answer.Transport.SDP, "v=0") {
		t.Fatalf("no session id or SDP answer: %s", resp.Body)
	}
	t.Logf("vendor opened session %s; it never connects media and ends when ICE gives up", resp.ID)
}
