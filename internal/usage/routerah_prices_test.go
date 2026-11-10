package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchRouterahPricesUsesCorrectBillingUnits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[
			{"model_name":"text-id","type":"text","input_price":0.2,"output_price":0.6},
			{"model_name":"image-id","type":"image","is_fixed_price":true,"fixed_price":0.003},
			{"model_name":"free-id","type":"embedding","is_free":true},
			{"model_name":"video-id","type":"video","is_fixed_price":true,"fixed_price":0.5},
			{"model_name":" ","type":"text","input_price":1}
		]}`))
	}))
	defer srv.Close()
	got, err := FetchRouterahPrices(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if p := got["text-id"]; p.Input != 0.2 || p.Output != 0.6 {
		t.Errorf("text token prices = %+v", p)
	}
	if p := got["image-id"]; p.PerImage != 0.003 {
		t.Errorf("image fixed price = %+v", p)
	}
	if p, ok := got["free-id"]; !ok || p != (Price{}) {
		t.Errorf("free model price = %+v (ok %v)", p, ok)
	}
	if _, ok := got["video-id"]; ok {
		t.Error("unknown fixed-price units must not be priced as tokens or images")
	}
}

func TestFetchRouterahPricesRejectsBadCatalog(t *testing.T) {
	for _, test := range []struct {
		name string
		code int
		body string
	}{
		{"http failure", http.StatusBadGateway, "{}"},
		{"invalid json", http.StatusOK, "not-json"},
		{"empty", http.StatusOK, `{"models":[]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.code)
				_, _ = w.Write([]byte(test.body))
			}))
			defer srv.Close()
			if _, err := FetchRouterahPrices(context.Background(), srv.URL); err == nil {
				t.Fatal("expected catalogue failure")
			}
		})
	}
}

func TestTrackerDynamicProviderPricesAreIsolatedAndRefreshable(t *testing.T) {
	tracker := New(map[string]Price{
		"model-a":          {Input: 3, Output: 4},
		"routerah/model-a": {Input: 7, Output: 8},
	})
	models := map[string]Price{
		"model-a": {Input: 1, Output: 2},
		"model-b": {Input: 0.2, Output: 0.6},
	}
	tracker.SetProviderPrices("routerah", models)
	models["model-b"] = Price{Input: 100, Output: 100}
	for _, sample := range []struct {
		provider string
		model    string
		input    float64
	}{
		{"routerah", "model-a", 7},
		{"routerah", "model-b", 0.2},
		{"openrouter", "model-b", 0},
		{"openrouter", "model-a", 3},
	} {
		p, _ := tracker.Price(sample.provider, sample.model)
		if p.Input != sample.input {
			t.Errorf("%s/%s input rate = %v, want %v", sample.provider, sample.model, p.Input, sample.input)
		}
	}
	tracker.SetProviderPrices("routerah", map[string]Price{"model-b": {Input: 0.4}})
	if p, _ := tracker.Price("routerah", "model-b"); p.Input != 0.4 {
		t.Errorf("fresh rate was not applied: %+v", p)
	}
}
