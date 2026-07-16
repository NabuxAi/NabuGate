package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"nabugate/internal/agent"
	"nabugate/internal/photos"
	"nabugate/internal/policy"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

func newPhotoTestServer(t *testing.T, client *photos.Client) *Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	r := router.New(nil, nil, nil, nil, nil, nil, log)
	srv := New(r, policy.New(nil, nil), usage.New(nil), agent.NewRegistry(), log)
	if client != nil {
		srv.WithPhotos(client)
	}
	return srv
}

func TestPhotoSearchDisabled(t *testing.T) {
	srv := newPhotoTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/photos/search?query=cat", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when photos are unconfigured, got %d", rec.Code)
	}
}

func TestPhotoSearchEmptyQueryServesCurated(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/curated" {
			t.Errorf("expected curated endpoint without a query, got %q", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "" {
			t.Errorf("curated request must not carry a query, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"photos": [{"id": 7, "src": {"large": "https://img/c.jpg"}}], "page": 1, "per_page": 10, "total_results": 1}`))
	}))
	defer upstream.Close()
	srv := newPhotoTestServer(t, photos.New("test-key", upstream.URL))

	req := httptest.NewRequest(http.MethodGet, "/v1/photos/search", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 curated feed without query, got %d: %s", rec.Code, rec.Body.String())
	}
	var out photos.SearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Photos) != 1 || out.Photos[0].ID != 7 {
		t.Fatalf("unexpected curated result (err=%v): %+v", err, out)
	}
}

func TestPhotoSearchPassesSizeParam(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("size"); got != "large" {
			t.Errorf("expected size=large upstream, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"photos": [], "page": 1, "per_page": 1, "total_results": 0}`))
	}))
	defer upstream.Close()
	srv := newPhotoTestServer(t, photos.New("test-key", upstream.URL))

	req := httptest.NewRequest(http.MethodGet, "/v1/photos/search?query=cat&size=large", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestPhotoSearchProxiesAndNormalizes(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "test-key" {
			t.Errorf("expected Pexels key auth header, got %q", got)
		}
		if r.URL.Path != "/v1/search" {
			t.Errorf("unexpected upstream path %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("query") != "sunset beach" || q.Get("orientation") != "portrait" {
			t.Errorf("unexpected upstream query %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"photos": [{
				"id": 42, "width": 3000, "height": 4500,
				"url": "https://www.pexels.com/photo/42/",
				"photographer": "Jane", "photographer_url": "https://www.pexels.com/@jane",
				"avg_color": "#123456", "alt": "a sunset",
				"src": {"original": "https://img/o.jpg", "portrait": "https://img/p.jpg", "large2x": "https://img/l2.jpg"},
				"liked": false, "internal_field": "dropped"
			}],
			"page": 1, "per_page": 1, "total_results": 12345
		}`))
	}))
	defer upstream.Close()

	srv := newPhotoTestServer(t, photos.New("test-key", upstream.URL))
	req := httptest.NewRequest(http.MethodGet,
		"/v1/photos/search?query=sunset+beach&orientation=portrait&per_page=1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out photos.SearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if out.Provider != "pexels" || out.TotalResults != 12345 || len(out.Photos) != 1 {
		t.Fatalf("unexpected normalized result: %+v", out)
	}
	p := out.Photos[0]
	if p.ID != 42 || p.Photographer != "Jane" || p.Src.Portrait != "https://img/p.jpg" {
		t.Fatalf("unexpected photo fields: %+v", p)
	}
}

func TestPhotoSearchUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"quota exceeded"}`, http.StatusTooManyRequests)
	}))
	defer upstream.Close()

	srv := newPhotoTestServer(t, photos.New("test-key", upstream.URL))
	req := httptest.NewRequest(http.MethodGet, "/v1/photos/search?query=cat", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 on upstream failure, got %d", rec.Code)
	}
}
