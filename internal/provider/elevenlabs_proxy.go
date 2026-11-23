package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ElevenLabsProxy forwards one HTTP call to the ElevenLabs REST API.
//
// Speech and Transcribe on the adapter reshape a request into the OpenAI wire
// format, which drops the fields a product actually sends: audio tags,
// language_code, voice_settings, word timestamps, multipart video. This proxy
// does not reshape. The caller speaks ElevenLabs' paths and bodies; the proxy
// adds xi-api-key and nothing else.
type ElevenLabsProxy struct {
	origin string
	apiKey string
	client *http.Client
}

// newElevenLabsProxy builds the forwarder. baseURL is the adapter's, which
// historically includes /v1; the proxy talks to the origin and lets the caller
// pass /v1/... itself.
func newElevenLabsProxy(baseURL, apiKey string) *ElevenLabsProxy {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// A compressed body would no longer match the bytes ElevenLabs sent.
	transport.DisableCompression = true
	// Music, dubbing and long speech can sit a while before the first byte.
	// There is no overall timeout: a lip-sync upload is large, and cutting it
	// off by the clock would bill a partial file.
	transport.ResponseHeaderTimeout = 15 * time.Minute
	return &ElevenLabsProxy{
		origin: elevenOrigin(baseURL),
		apiKey: apiKey,
		client: &http.Client{
			Transport: transport,
			// A redirect would carry xi-api-key to whatever host answered.
			// The caller can follow a redirect itself, without the key.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// elevenOrigin strips a trailing /v1 so /elevenlabs/v1/... maps onto the
// vendor's /v1/... exactly once.
func elevenOrigin(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	base = strings.TrimSuffix(base, "/v1")
	if base == "" {
		return "https://api.elevenlabs.io"
	}
	return base
}

// GatewayKey is the deployment's ElevenLabs key. It is sent as xi-api-key and
// never written on a response.
func (p *ElevenLabsProxy) GatewayKey() string {
	if p == nil {
		return ""
	}
	return p.apiKey
}

// Forward sends one request and returns the upstream response. The body is
// streamed. apiKey overrides the deployment key when the caller brought their
// own; an empty apiKey uses the deployment key. The response body is the
// caller's to close.
func (p *ElevenLabsProxy) Forward(ctx context.Context, method, path, rawQuery string, header http.Header, body io.Reader, contentLength int64, apiKey string) (*http.Response, error) {
	if p == nil {
		return nil, io.ErrUnexpectedEOF
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		key = p.apiKey
	}
	endpoint, err := url.Parse(p.origin)
	if err != nil {
		return nil, err
	}
	// Keep a path the operator put on base_url. Replacing Path would turn
	// https://proxy.example/vendor/elevenlabs into https://proxy.example/v1/...
	// and skip the prefix the speech adapter still uses.
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	endpoint.RawQuery = rawQuery
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	copyElevenRequestHeaders(req.Header, header)
	if contentLength > 0 {
		req.ContentLength = contentLength
	}
	req.Header.Set("xi-api-key", key)
	return p.client.Do(req)
}

// Headers a client must not be able to use to point the upstream at itself,
// and headers that would leak the gateway key back if echoed.
var elevenDropRequestHeaders = map[string]bool{
	"authorization":       true,
	"xi-api-key":          true,
	"host":                true,
	"cookie":              true,
	"content-length":      true,
	"transfer-encoding":   true,
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailer":             true,
	"upgrade":             true,
}

func copyElevenRequestHeaders(dst, src http.Header) {
	for name, values := range src {
		lower := strings.ToLower(name)
		if elevenDropRequestHeaders[lower] || strings.HasPrefix(lower, "x-nabu-") {
			continue
		}
		for _, v := range values {
			dst.Add(name, v)
		}
	}
}

// ElevenUpstreamPath maps a gateway path (/elevenlabs/v1/...) onto the vendor
// path. ok is false for anything that is not under that prefix, or that tries
// to leave it. /v1/speech-to-speech/ is the name callers use for the voice
// changer; ElevenLabs serves that API at /v1/voice-changer/.
func ElevenUpstreamPath(gatewayPath string) (string, bool) {
	const prefix = "/elevenlabs"
	if gatewayPath != prefix && !strings.HasPrefix(gatewayPath, prefix+"/") {
		return "", false
	}
	path := strings.TrimPrefix(gatewayPath, prefix)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, "..") {
		return "", false
	}
	path = strings.Replace(path, "/v1/speech-to-speech/", "/v1/voice-changer/", 1)
	return path, true
}

// elevenRoots are the vendor path prefixes this gateway will spend its key on.
// Account, workspace and API-key routes are absent on purpose: they would let
// a tenant administer the shared ElevenLabs account.
var elevenRoots = []string{
	"/v1/text-to-speech",
	"/v1/text-to-dialogue",
	"/v1/speech-to-text",
	"/v1/voice-changer",
	"/v1/sound-generation",
	"/v1/music",
	"/v1/dubbing",
	"/v1/audio-isolation",
	"/v1/forced-alignment",
	"/v1/audio-native",
	"/v1/voices",
	"/v2/voices",
	"/v1/shared-voices",
	"/v1/similar-voices",
	"/v1/text-to-voice",
	"/v1/models",
	"/v1/assets",
	"/v1/flows/video",
	"/v1/audio-effects",
	"/v1/pronunciation-dictionaries",
}

// ElevenPathAllowed reports whether method+path is one this gateway proxies.
// path is the upstream path, without a query string.
func ElevenPathAllowed(method, path string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
	default:
		return false
	}
	path = strings.Split(path, "?")[0]
	for _, root := range elevenRoots {
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

// ElevenUsage is what one proxied call costs, in the units the price table
// already knows. Charge is false for reads, for polls that are not the
// billing moment, and for upstream errors. Measured is false when a call
// clearly did work but the quantity was not in the request or the response —
// the gateway records that as unpriced rather than as free.
type ElevenUsage struct {
	Model      string
	Characters int
	Seconds    float64
	JobID      string
	Charge     bool
	Measured   bool
	Estimated  bool
	Family     string
}

// MeasureEleven decides what a proxied call consumed.
//
// Request JSON is enough for speech, music and sound effects. Transcription,
// dubbing and forced alignment put the duration in the response. A lip-sync job
// is billed from duration_secs in that JSON, which is the model's own field.
// A caller-supplied duration header is not a measurement: the tenant chooses
// it. Uploaded audio is measured separately, from the file, and applied with
// ApplyUpload. respStatus 0 means the response is not known yet.
func MeasureEleven(method, upstreamPath, reqContentType string, reqBody []byte, respStatus int, respContentType string, respBody []byte) ElevenUsage {
	path := strings.Split(upstreamPath, "?")[0]
	family := elevenFamily(path)
	u := ElevenUsage{Family: family, Model: elevenDefaultModel(family, path)}

	var req map[string]any
	if strings.Contains(strings.ToLower(reqContentType), "json") && len(reqBody) > 0 {
		_ = json.Unmarshal(reqBody, &req)
	}
	if model := elevenModelOf(req); model != "" {
		u.Model = model
	}
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		applyElevenRequest(&u, req)
	}

	if respStatus == 0 {
		return u
	}
	if respStatus < 200 || respStatus >= 300 {
		u.Charge = false
		u.Measured = false
		return u
	}

	var resp map[string]any
	if strings.Contains(strings.ToLower(respContentType), "json") && json.Valid(respBody) {
		_ = json.Unmarshal(respBody, &resp)
	}
	if id := elevenJobID(resp); id != "" {
		u.JobID = id
	}
	// Multipart requests don't carry model_id in a JSON body. Use one the
	// response names (Scribe does, on some payloads) and otherwise keep the
	// family default.
	if elevenModelOf(req) == "" {
		if model := elevenModelOf(resp); model != "" {
			u.Model = model
		}
	}
	status := elevenStatus(resp)
	if secs := elevenResponseSeconds(resp); secs > 0 && elevenBilledInSeconds(family) {
		u.Seconds = secs
		u.Measured = true
		u.Estimated = false
		u.Charge = true
	}
	if method == http.MethodGet && elevenInProgress(status) {
		u.Charge = false
	}
	if elevenFailed(status) {
		u.Charge = false
		u.Measured = false
	}
	// An accepted async job with no duration yet is billed when a later poll
	// returns one. A synchronous audio response (voice changer, isolator) will
	// not grow a duration later, so an unmeasured 200 is recorded as unpriced.
	if u.Charge && !u.Measured && elevenAsync(family) && (status == "" || elevenInProgress(status) || method == http.MethodGet) {
		u.Charge = false
	}
	return u
}

func elevenFamily(path string) string {
	switch {
	case strings.Contains(path, "/text-to-speech/"), strings.Contains(path, "/text-to-dialogue"):
		return "tts"
	case strings.Contains(path, "/speech-to-text"):
		return "stt"
	case strings.Contains(path, "/voice-changer/"):
		return "sts"
	case strings.HasPrefix(path, "/v1/sound-generation"):
		return "sfx"
	case strings.HasPrefix(path, "/v1/music"):
		// A composition plan, an upload and a finetune record are not minutes
		// of generated music. The compose and stream routes are.
		if strings.Contains(path, "/finetunes") || strings.HasSuffix(path, "/plan") || strings.HasSuffix(path, "/upload") {
			return "other"
		}
		return "music"
	case strings.HasPrefix(path, "/v1/dubbing"):
		return "dub"
	case strings.HasPrefix(path, "/v1/audio-isolation"):
		return "iso"
	case strings.HasPrefix(path, "/v1/forced-alignment"):
		return "align"
	case strings.HasPrefix(path, "/v1/text-to-voice"):
		return "design"
	case strings.HasPrefix(path, "/v1/flows/video"):
		return "video"
	case strings.HasPrefix(path, "/v1/audio-native"):
		return "native"
	default:
		return "other"
	}
}

func elevenDefaultModel(family, path string) string {
	switch family {
	case "tts", "native":
		return "eleven_multilingual_v2"
	case "design":
		return "eleven_multilingual_ttv_v2"
	case "stt":
		return "scribe_v2"
	case "sts":
		return "eleven_multilingual_sts_v2"
	case "sfx":
		return "eleven_text_to_sound_v2"
	case "music":
		return "music_v1"
	case "iso":
		return "eleven_audio_isolation"
	case "align":
		return "forced-alignment"
	case "dub":
		if strings.Contains(path, "/dubbing/project") {
			return "dubbing_v2"
		}
		return "dubbing"
	case "video":
		return "sync-lipsync-v3"
	default:
		return ""
	}
}

func elevenBilledInSeconds(family string) bool {
	switch family {
	case "stt", "sts", "sfx", "music", "dub", "iso", "align", "video":
		return true
	default:
		return false
	}
}

func elevenAsync(family string) bool {
	switch family {
	case "video", "dub", "stt":
		return true
	default:
		return false
	}
}

func applyElevenRequest(u *ElevenUsage, body map[string]any) {
	switch u.Family {
	case "tts":
		n := elevenSpokenChars(body)
		if n > 0 {
			u.Characters = n
			u.Charge = true
			u.Measured = true
		}
	case "native":
		// The document is usually a multipart file, so the JSON body has no
		// character count. Charge stays on so a conversion we could not read
		// is recorded unpriced rather than treated as free. ApplyUpload fills
		// the count when the file part was text.
		n := elevenSpokenChars(body)
		u.Charge = true
		if n > 0 {
			u.Characters = n
			u.Measured = true
		}
	case "design":
		// Saving a preview (POST /v1/text-to-voice with a generated_voice_id and
		// no preview text) does not synthesize. Designing one does.
		if body == nil {
			return
		}
		if _, saving := body["generated_voice_id"]; saving && elevenSpokenChars(body) == 0 {
			return
		}
		if !strings.Contains(u.Model, "ttv") && body["voice_description"] == nil && body["text"] == nil {
			return
		}
		n := elevenSpokenChars(body)
		if n == 0 {
			// The vendor writes the preview script when the caller does not.
			// 1000 characters is the floor documented for that case, not a
			// measurement — Estimated says so.
			n = 1000
			u.Estimated = true
		}
		u.Characters = n
		u.Charge = true
		u.Measured = true
	case "sfx":
		if secs, ok := elevenRequestSeconds(body); ok {
			u.Seconds = secs
		} else {
			u.Seconds = 10
			u.Estimated = true
		}
		u.Charge = true
		u.Measured = true
	case "music":
		if secs, ok := elevenRequestSeconds(body); ok {
			u.Seconds = secs
		} else {
			u.Seconds = 30
			u.Estimated = true
		}
		u.Charge = true
		u.Measured = true
	case "stt", "sts", "dub", "iso", "align", "video":
		if secs, ok := elevenRequestSeconds(body); ok {
			u.Seconds = secs
			u.Charge = true
			u.Measured = true
		} else {
			u.Charge = true
			u.Measured = false
		}
	}
}

// ApplyUpload adds a quantity read from the uploaded file itself: seconds from
// a WAV or MP3, or characters from an Audio Native document. It never replaces
// a quantity the vendor's own JSON already named.
func (u *ElevenUsage) ApplyUpload(seconds float64, characters int, secondsEstimated bool) {
	if u == nil || !u.Charge || u.Measured {
		return
	}
	if characters > 0 && (u.Family == "native" || u.Family == "tts") {
		u.Characters = characters
		u.Measured = true
		u.Estimated = false
		return
	}
	if seconds > 0 && elevenBilledInSeconds(u.Family) {
		u.Seconds = seconds
		u.Measured = true
		u.Estimated = secondsEstimated
	}
}

// ElevenMeasuresUpload reports whether the request body is the thing being
// billed and is not JSON: voice changer, isolator, and Audio Native.
func ElevenMeasuresUpload(upstreamPath string) bool {
	switch elevenFamily(strings.Split(upstreamPath, "?")[0]) {
	case "sts", "iso", "native":
		return true
	default:
		return false
	}
}

func elevenSpokenChars(body map[string]any) int {
	if body == nil {
		return 0
	}
	if s, ok := body["text"].(string); ok && strings.TrimSpace(s) != "" {
		return utf8.RuneCountInString(s)
	}
	n := 0
	for _, key := range []string{"inputs", "dialogue"} {
		arr, ok := body[key].([]any)
		if !ok {
			continue
		}
		for _, item := range arr {
			im, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if s, ok := im["text"].(string); ok {
				n += utf8.RuneCountInString(s)
			}
		}
	}
	if n == 0 {
		if s, ok := body["content"].(string); ok {
			n = utf8.RuneCountInString(s)
		}
	}
	return n
}

func elevenModelOf(body map[string]any) string {
	if body == nil {
		return ""
	}
	for _, key := range []string{"model_id", "model"} {
		if s, ok := body[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func elevenRequestSeconds(body map[string]any) (float64, bool) {
	if body == nil {
		return 0, false
	}
	for _, key := range []string{"duration_seconds", "duration_secs", "duration"} {
		if f, ok := elevenNumber(body[key]); ok && f > 0 {
			return f, true
		}
	}
	if f, ok := elevenNumber(body["music_length_ms"]); ok && f > 0 {
		return f / 1000, true
	}
	if plan, ok := body["composition_plan"].(map[string]any); ok {
		if ms := elevenPlanMillis(plan); ms > 0 {
			return ms / 1000, true
		}
	}
	if end, ok := elevenNumber(body["end_time"]); ok && end > 0 {
		start, _ := elevenNumber(body["start_time"])
		if end > start {
			return end - start, true
		}
	}
	return 0, false
}

// elevenPlanMillis is the length of a music composition plan, which has no
// music_length_ms: music_v1 lists sections and music_v2 lists chunks, each
// with its own duration_ms.
func elevenPlanMillis(plan map[string]any) float64 {
	total := 0.0
	for _, key := range []string{"sections", "chunks"} {
		items, _ := plan[key].([]any)
		for _, item := range items {
			part, _ := item.(map[string]any)
			if ms, ok := elevenNumber(part["duration_ms"]); ok && ms > 0 {
				total += ms
			}
		}
	}
	return total
}

func elevenNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func elevenJobID(body map[string]any) string {
	if body == nil {
		return ""
	}
	for _, key := range []string{"dubbing_id", "transcription_id", "generation_id", "project_id", "id"} {
		if s, ok := body[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func elevenStatus(body map[string]any) string {
	if body == nil {
		return ""
	}
	s, _ := body["status"].(string)
	return strings.ToLower(strings.TrimSpace(s))
}

func elevenInProgress(status string) bool {
	switch status {
	case "pending", "generating", "queued", "preparing", "dubbing", "cloning", "processing", "ready":
		return true
	default:
		return false
	}
}

func elevenFailed(status string) bool {
	switch status {
	case "failed", "error", "rejected":
		return true
	default:
		return false
	}
}

func elevenResponseSeconds(body map[string]any) float64 {
	if body == nil {
		return 0
	}
	for _, key := range []string{"audio_duration_secs", "expected_duration_sec", "duration_seconds", "duration_secs"} {
		if f, ok := elevenNumber(body[key]); ok && f > 0 {
			return f
		}
	}
	if md, ok := body["media_metadata"].(map[string]any); ok {
		if f, ok := elevenNumber(md["duration"]); ok && f > 0 {
			return f
		}
	}
	if secs := elevenWordEnd(body["words"]); secs > 0 {
		return secs
	}
	if secs := elevenWordEnd(body["characters"]); secs > 0 {
		return secs
	}
	if arr, ok := body["transcripts"].([]any); ok {
		var max float64
		for _, item := range arr {
			im, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if secs := elevenResponseSeconds(im); secs > max {
				max = secs
			}
		}
		return max
	}
	return 0
}

func elevenWordEnd(v any) float64 {
	arr, ok := v.([]any)
	if !ok {
		return 0
	}
	var max float64
	for _, item := range arr {
		im, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if f, ok := elevenNumber(im["end"]); ok && f > max {
			max = f
		}
	}
	return max
}

// ReadElevenJSONBody buffers a JSON request so it can be metered and then
// forwarded. Anything that is not JSON, including multipart video, is left
// unread (ok=false) and must be streamed. Bodies over the limit are refused.
func ReadElevenJSONBody(r io.Reader, contentType string, limit int64) (body []byte, buffered bool, err error) {
	if !strings.Contains(strings.ToLower(contentType), "json") {
		return nil, false, nil
	}
	if limit <= 0 {
		limit = 48 << 20
	}
	buf, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, true, err
	}
	if int64(len(buf)) > limit {
		return nil, true, bytes.ErrTooLarge
	}
	return buf, true, nil
}
