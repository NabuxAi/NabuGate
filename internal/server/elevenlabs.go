package server

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"nabugate/internal/adminstore"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// maxElevenLabsBody is the largest upload the proxy will read. Forced alignment
// and dubbing accept up to about a gigabyte at the vendor; the chat cap (16 MiB)
// would reject a real video before ElevenLabs had a chance to.
const maxElevenLabsBody = 1 << 30

// maxElevenJSON is how much of a JSON body is held in memory so it can be
// metered and then forwarded. Inline images on the video API are capped at
// 25 MiB decoded, which is larger than that in base64, so this sits above it.
// Multipart video is never buffered; it is streamed.
const maxElevenJSON = 48 << 20

// elevenResponseBuffer is how much of a JSON response is held to read a
// duration out of it. A transcript with word timestamps fits; past this the
// rest is still streamed to the client and metering falls back to the request.
const maxElevenResponseJSON = 32 << 20

// elevenBills is the set of async job ids already charged for a project.
type elevenBills struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func newElevenBills() *elevenBills {
	return &elevenBills{seen: map[string]struct{}{}}
}

// claim reports whether this is the first time id is billed. A poll of a job
// that was billed on create gets false. The map is capped so a long-lived
// process cannot grow it without bound; past the cap the oldest entries are
// dropped by resetting, which can bill one late poll a second time.
func (b *elevenBills) claim(id string) bool {
	if b == nil || id == "" {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.seen[id]; ok {
		return false
	}
	if len(b.seen) > 10000 {
		b.seen = map[string]struct{}{}
	}
	b.seen[id] = struct{}{}
	return true
}

func (s *Server) elevenProxy() *provider.ElevenLabsProxy {
	if s.router == nil {
		return nil
	}
	adapter, ok := s.router.Adapter("elevenlabs")
	if !ok {
		return nil
	}
	el, ok := adapter.(*provider.ElevenLabsAdapter)
	if !ok {
		return nil
	}
	return el.Proxy()
}

// handleElevenLabs proxies /elevenlabs/* to the ElevenLabs REST API.
//
// The path under the prefix is the vendor's path, unchanged, and the body is
// forwarded unchanged. What the gateway adds is the API key, the caller's
// project auth and rate limit, and a usage row. Upstream status codes and
// bodies — including errors — are returned as ElevenLabs sent them.
func (s *Server) handleElevenLabs(w http.ResponseWriter, r *http.Request) {
	proxy := s.elevenProxy()
	upstream, ok := provider.ElevenUpstreamPath(r.URL.Path)
	if !ok || strings.Contains(upstream, "..") {
		writeError(w, http.StatusNotFound, "unknown ElevenLabs path")
		return
	}
	if !provider.ElevenPathAllowed(r.Method, upstream) {
		// Not proxied, and not forwarded: account and API-key routes on the
		// shared vendor account are not a tenant feature.
		s.log.Warn("elevenlabs path refused", "project", s.project(r), "method", r.Method, "path", upstream)
		writeError(w, http.StatusNotFound, "that ElevenLabs path is not proxied")
		return
	}
	if !s.aliasAllowed(w, r, "elevenlabs") {
		return
	}
	if !elevenProviderAllowed(r) {
		writeError(w, http.StatusForbidden, "provider \"elevenlabs\" is not permitted for this key")
		return
	}
	// No adapter means this deployment has no ElevenLabs origin to forward to.
	// A caller's own key still needs that origin, so it does not bypass this.
	if proxy == nil {
		writeError(w, http.StatusServiceUnavailable, "ElevenLabs is not configured (set ELEVENLABS_API_KEY)")
		return
	}

	apiKey, byCaller, code, msg := s.elevenCredential(r, proxy)
	if code != 0 {
		writeError(w, code, msg)
		return
	}
	if rec, _ := r.Context().Value(router.KeySourceCtxKey{}).(*router.KeySource); rec != nil {
		if byCaller {
			rec.Set("caller")
		} else {
			rec.Set("gateway")
		}
	}

	reqBody, reqCT, err := readElevenRequest(r)
	if err != nil {
		if errors.Is(err, bytes.ErrTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "JSON body exceeds the ElevenLabs proxy limit")
			return
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "upload exceeds the ElevenLabs proxy limit")
			return
		}
		writeError(w, http.StatusBadRequest, "could not read the request body")
		return
	}

	durationHeader := provider.ElevenDurationHeader(r.Header)
	var body io.Reader
	var contentLength int64
	if reqBody != nil {
		body = bytes.NewReader(reqBody)
		contentLength = int64(len(reqBody))
	} else {
		body = r.Body
		contentLength = r.ContentLength
	}

	resp, err := proxy.Forward(r.Context(), r.Method, upstream, r.URL.RawQuery, r.Header, body, contentLength, apiKey)
	if err != nil {
		s.log.Warn("elevenlabs upstream unreachable", "project", s.project(r), "method", r.Method, "path", upstream, "error", err)
		writeError(w, http.StatusBadGateway, "ElevenLabs did not answer")
		return
	}
	defer resp.Body.Close()

	respCT := resp.Header.Get("Content-Type")
	respBody, streamed, err := readElevenResponse(resp)
	if err != nil {
		s.log.Warn("elevenlabs response unreadable", "project", s.project(r), "path", upstream, "error", err)
		writeError(w, http.StatusBadGateway, "ElevenLabs response could not be read")
		return
	}

	var measuredBody []byte
	if reqBody != nil {
		measuredBody = reqBody
	}
	bill := provider.MeasureEleven(r.Method, upstream, reqCT, measuredBody, durationHeader, resp.StatusCode, respCT, respBody)
	cost := s.chargeEleven(r, bill)

	copyElevenResponseHeaders(w.Header(), resp.Header)
	w.Header().Set("X-Nabu-Provider", "elevenlabs")
	if bill.Model != "" {
		w.Header().Set("X-Nabu-Model", bill.Model)
	}
	w.Header().Set(costHeader, formatCost(cost))
	w.WriteHeader(resp.StatusCode)

	if streamed != nil {
		_, _ = w.Write(respBody)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		buf := make([]byte, 32*1024)
		for {
			n, readErr := streamed.Read(buf)
			if n > 0 {
				_, _ = w.Write(buf[:n])
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
			if readErr != nil {
				break
			}
		}
	} else if len(respBody) > 0 {
		_, _ = w.Write(respBody)
	}

	s.log.Info("elevenlabs", "project", s.project(r), "method", r.Method, "path", upstream,
		"status", resp.StatusCode, "model", bill.Model, "cost_usd", cost,
		"characters", bill.Characters, "seconds", bill.Seconds, "measured", bill.Measured, "by_caller", byCaller)
}

// readElevenRequest buffers a JSON body and leaves everything else to be streamed.
func readElevenRequest(r *http.Request) (body []byte, contentType string, err error) {
	contentType = r.Header.Get("Content-Type")
	if r.Body == nil || r.Method == http.MethodGet || r.Method == http.MethodHead {
		return nil, contentType, nil
	}
	buffered, ok, err := provider.ReadElevenJSONBody(r.Body, contentType, maxElevenJSON)
	if err != nil || !ok {
		return nil, contentType, err
	}
	return buffered, contentType, nil
}

// readElevenResponse buffers a JSON response up to maxElevenResponseJSON so a
// duration can be read off it. Past that, the prefix is returned and the
// remainder is left in streamed for the handler to copy. Audio and video are
// never buffered.
func readElevenResponse(resp *http.Response) (prefix []byte, streamed io.Reader, err error) {
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(ct, "json") {
		return nil, resp.Body, nil
	}
	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxElevenResponseJSON+1))
	if err != nil {
		return nil, nil, err
	}
	if len(buf) <= maxElevenResponseJSON {
		return buf, nil, nil
	}
	return buf[:maxElevenResponseJSON], io.MultiReader(bytes.NewReader(buf[maxElevenResponseJSON:]), resp.Body), nil
}

func copyElevenResponseHeaders(dst, src http.Header) {
	hop := map[string]bool{
		"connection": true, "keep-alive": true, "proxy-authenticate": true,
		"proxy-authorization": true, "te": true, "trailer": true,
		"transfer-encoding": true, "upgrade": true, "xi-api-key": true,
	}
	for name, values := range src {
		if hop[strings.ToLower(name)] {
			continue
		}
		for _, v := range values {
			dst.Add(name, v)
		}
	}
}

// chargeEleven records one proxied call and returns what the caller was charged.
// A job id that was already billed (the create, or an earlier poll) is not
// billed again. An unmeasured charge is recorded as unpriced so a zero is not
// mistaken for a free call.
func (s *Server) chargeEleven(r *http.Request, bill provider.ElevenUsage) float64 {
	if !bill.Charge {
		return 0
	}
	if bill.Measured && bill.JobID != "" {
		if !s.elevenBills.claim(s.project(r) + "|" + bill.JobID) {
			return 0
		}
	}
	metered := usage.Metered{Characters: bill.Characters, Seconds: bill.Seconds}
	if bill.Estimated {
		metered.Tokens.Estimated = true
	}
	if !bill.Measured {
		s.meter(r, s.project(r), "elevenlabs", bill.Model, adminstore.UsageEntry{
			Requests: 1,
			Unpriced: !servedByCaller(r.Context()),
		})
		s.log.Warn("elevenlabs call metered at nothing: no duration or character count on the request or the response",
			"project", s.project(r), "model", bill.Model, "family", bill.Family)
		return 0
	}
	return s.recordMetered(r, "elevenlabs", bill.Model, metered)
}

// elevenCredential picks the key that will be sent to ElevenLabs.
//
// The deployment key is the default. A caller's own key (the X-Nabu-Key-elevenlabs
// header, or one saved on their account) is used when the key mode says so, and
// then the call is not billed — their vendor already billed them. A provider
// held at access:request refuses the deployment key until the account is allowed
// to spend it.
func (s *Server) elevenCredential(r *http.Request, proxy *provider.ElevenLabsProxy) (key string, byCaller bool, code int, msg string) {
	gateway := ""
	if proxy != nil {
		gateway = proxyGatewayKey(proxy)
	}
	creds, _ := r.Context().Value(router.CallerKeysCtxKey{}).(router.CallerKeys)
	creds = creds.Normalize()
	var own string
	if keys := creds.Keys["elevenlabs"]; len(keys) > 0 {
		own = keys[0]
	}
	denied := false
	if m, ok := r.Context().Value(router.GatewayDeniedCtxKey{}).(map[string]bool); ok {
		denied = m["elevenlabs"]
	}
	useGateway := func() (string, bool, int, string) {
		if denied {
			return "", false, http.StatusForbidden, "using the gateway's key for this provider needs approval — request it in the console, or send your own key"
		}
		if gateway == "" {
			return "", false, http.StatusServiceUnavailable, "ElevenLabs is not configured (set ELEVENLABS_API_KEY)"
		}
		return gateway, false, 0, ""
	}
	switch creds.Mode {
	case router.ModeOwn:
		if own == "" {
			return "", false, http.StatusBadRequest, "X-Nabu-Key-Mode is own but no X-Nabu-Key-elevenlabs was sent"
		}
		return own, true, 0, ""
	case router.ModeGlobal:
		return useGateway()
	case router.ModeGlobalFirst:
		if !denied && gateway != "" {
			return gateway, false, 0, ""
		}
		if own != "" {
			return own, true, 0, ""
		}
		return useGateway()
	default: // own-first
		if own != "" {
			return own, true, 0, ""
		}
		return useGateway()
	}
}

func elevenProviderAllowed(r *http.Request) bool {
	allowed, ok := r.Context().Value(router.AllowedProvidersCtxKey{}).([]string)
	if !ok || len(allowed) == 0 {
		return true
	}
	for _, name := range allowed {
		if strings.EqualFold(name, "elevenlabs") {
			return true
		}
	}
	return false
}

// proxyGatewayKey reads the deployment key without exporting it on the proxy
// type's usual surface. Tests and the handler both need it; the response path
// never writes it back.
func proxyGatewayKey(p *provider.ElevenLabsProxy) string {
	if p == nil {
		return ""
	}
	return p.GatewayKey()
}
