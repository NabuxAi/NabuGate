package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRouterahPublicCatalogListsAllModelTypesAndDeduplicates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pricing/catalog" {
			t.Errorf("catalogue path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing authorization")
		}
		_, _ = w.Write([]byte(`{"counts":{"models":5},"models":[
			{"model_name":"model-text","type":"text"},
			{"model_name":"provider/image","type":"image"},
			{"model_name":"audio-model","type":"audio"},
			{"model_name":"video-model","type":"video"},
			{"model_name":"embedding:free","type":"embedding"},
			{"model_name":"model-text","type":"text"},
			{"model_name":"  ","type":"text"}
		]}`))
	}))
	defer srv.Close()

	adapter := NewOpenAIAdapter("routerah", srv.URL+"/v1", "test-key", nil)
	if err := adapter.SetModelsURL(srv.URL + "/api/pricing/catalog"); err != nil {
		t.Fatal(err)
	}
	models, err := adapter.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"model-text", "provider/image", "audio-model", "video-model", "embedding:free"}
	if !reflect.DeepEqual(models, want) {
		t.Errorf("models = %v; want %v", models, want)
	}
}

func TestOpenAIModelsURLCannotSendVendorKeyToAnotherOrigin(t *testing.T) {
	adapter := NewOpenAIAdapter("routerah", "https://api.routerah.app/v1", "secret", nil)
	for _, modelsURL := range []string{
		"https://unrelated.example/api/pricing/catalog",
		"http://api.routerah.app/api/pricing/catalog",
		"https://user:pass@api.routerah.app/api/pricing/catalog",
		"//api.routerah.app/api/pricing/catalog",
		"/api/pricing/catalog",
	} {
		if err := adapter.SetModelsURL(modelsURL); err == nil {
			t.Errorf("accepted foreign or ambiguous models_url %q", modelsURL)
		}
	}
	if err := adapter.SetModelsURL("https://api.routerah.app/api/pricing/catalog"); err != nil {
		t.Errorf("rejected same-origin catalogue: %v", err)
	}
}
