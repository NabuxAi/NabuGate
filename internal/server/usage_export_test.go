package server

import (
	"context"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nabugate/internal/adminstore"
)

// The CSV export is the operator's join key between what customers were charged
// and what the vendors billed: both costs, the honesty counters, and the model's
// list-price identity — one row per model, sorted, so a diff of two exports is
// meaningful. A non-admin must not be able to walk away with the whole
// deployment's spend, so the route is wrapped in requireAdmin and the handler
// itself is tested through that wrapper.
func TestUsageExportStreamsCSVToAdminsOnly(t *testing.T) {
	store, err := adminstore.Open(t.TempDir() + "/admin.json")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	store.RecordUsage("proj", "openai", "gpt-4o", adminstore.UsageEntry{
		Requests: 2, PromptTokens: 100, CompletionTokens: 50,
		CostUSD: 0.5, ProviderCostUSD: 0.3,
	})
	store.RecordUsage("proj", "openai", "gpt-4o", adminstore.UsageEntry{
		Requests: 1, PromptTokens: 10, CompletionTokens: 5,
		CostUSD: 0.1, ProviderCostUSD: 0.06, Unpriced: true, Estimated: true,
	})
	store.RecordUsage("proj", "deepgram", "nova-3", adminstore.UsageEntry{
		Requests: 1, CostUSD: 0.01, ProviderCostUSD: 0.0052, AudioSeconds: 90.5,
	})

	s := &Server{admin: store}
	h := requireAdmin(s.exportUsage)

	ask := func(admin bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/api/usage/export", nil)
		ctx := context.WithValue(r.Context(), consoleEmailCtxKey{}, "op@example.com")
		ctx = context.WithValue(ctx, consoleAdminCtxKey{}, admin)
		w := httptest.NewRecorder()
		h(w, r.WithContext(ctx))
		return w
	}

	if w := ask(false); w.Code != http.StatusForbidden {
		t.Errorf("non-admin export status %d, want 403", w.Code)
	}

	w := ask(true)
	if w.Code != http.StatusOK {
		t.Fatalf("admin export status %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content type %q, want text/csv", ct)
	}

	rows, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows (header + models), want 3:\n%s", len(rows), w.Body.String())
	}

	wantHeader := "model,identity,requests,prompt_tokens,completion_tokens,cost_usd,provider_cost_usd," +
		"unpriced_requests,estimated_requests,failed_requests,audio_seconds,images,characters"
	if rows[0][0] != "model" || strings.Join(rows[0], ",") != wantHeader {
		t.Errorf("header row is %q, want %q", strings.Join(rows[0], ","), wantHeader)
	}

	// Rows are sorted by the model key (the raw upstream name), so gpt-4o
	// sorts before nova-3 regardless of which was recorded first.
	if rows[1][0] != "gpt-4o" || rows[1][1] != "gpt-4o" {
		t.Errorf("gpt-4o row name/identity is %q", strings.Join(rows[1], ","))
	}
	// Both recorded entries aggregate into one row: 3 requests, and the two
	// costs carry six decimals so a spreadsheet reads them as money.
	if rows[1][2] != "3" || rows[1][3] != "110" || rows[1][4] != "55" {
		t.Errorf("gpt-4o counts are %q", strings.Join(rows[1], ","))
	}
	if rows[1][5] != "0.600000" || rows[1][6] != "0.360000" {
		t.Errorf("gpt-4o costs are %q", strings.Join(rows[1], ","))
	}
	if rows[1][7] != "1" || rows[1][8] != "1" || rows[1][9] != "0" {
		t.Errorf("gpt-4o honesty counters are %q", strings.Join(rows[1], ","))
	}
	if rows[2][0] != "nova-3" || rows[2][1] != "nova-3" || rows[2][10] != "90.5" {
		t.Errorf("nova-3 row is %q", strings.Join(rows[2], ","))
	}
}
