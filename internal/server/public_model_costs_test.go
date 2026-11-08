package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

func TestPublicModelCostsExposeOnlyPricedRateRangesAndPlanTerms(t *testing.T) {
	up := fakeUpstream(t)
	defer up.Close()

	adapters := map[string]provider.Adapter{
		"first": provider.NewOpenAIAdapter("first", up.URL, "dummy", nil),
		"second": provider.NewOpenAIAdapter("second", up.URL, "dummy", nil),
	}
	routes := map[string]config.ModelRoute{
		"nabu-smart": {
			Primary: config.Target{Provider: "first", Model: "model-a"},
			Fallback: []config.Target{{Provider: "second", Model: "model-b"}},
		},
	}
	r := router.New(adapters, routes, nil, nil, nil, nil, map[string][]string{}, discardLogger())
	s := New(r, policy.New(nil, nil), usage.New(map[string]usage.Price{
		"first/model-a": {Input: 2, Output: 10, InputCacheRead: 0.1},
	}), nil, discardLogger())
	s.SetPlans([]config.Plan{{ID:"gateway", Name:"Gateway access", PriceUSD:9, Days:30, Markup:1.4, MinChargeUSD:0.0002}})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/api/public/model-costs")
	if err != nil { t.Fatal(err) }
	defer res.Body.Close()
	if res.StatusCode != 200 { t.Fatalf("status=%d", res.StatusCode) }
	var got struct {
		Source string `json:"source"`
		Models []publicModelCosts `json:"models"`
		Plans []publicCostPlan `json:"plans"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil { t.Fatal(err) }
	if got.Source != "deployed_config" { t.Fatalf("source=%q",got.Source) }
	if len(got.Models)!=1 { t.Fatalf("models=%v",got.Models) }
	m:=got.Models[0]
	if m.Alias != "nabu-smart" || m.Complete || m.TotalRoutes != 2 || m.PricedRoutes != 1 {
		t.Fatalf("pricing completeness not disclosed: %+v",m)
	}
	if len(m.Rates)!=1 || m.Rates[0].Input!=2 || m.Rates[0].Output!=10 || m.Rates[0].InputCacheRead!=0.1 {
		t.Fatalf("configured rates not reflected: %+v",m.Rates)
	}
	if len(got.Plans)!=1 || got.Plans[0].Markup!=1.4 || got.Plans[0].MinChargeUSD != 0.0002 || got.Plans[0].PriceUSD!=9 {
		t.Fatalf("access fee and variable markup missing: %+v",got.Plans)
	}
}

func TestPublicModelCostsWithoutModelPricingIsNotClaimedFree(t *testing.T) {
	up:=fakeUpstream(t)
	defer up.Close()
	r:=router.New(map[string]provider.Adapter{
		"first": provider.NewOpenAIAdapter("first",up.URL,"dummy",nil),
	}, map[string]config.ModelRoute{
		"nabu-fast": {Primary: config.Target{Provider:"first",Model:"unknown"}},
	}, nil,nil,nil,nil,map[string][]string{},discardLogger())
	s:=New(r,policy.New(nil,nil),usage.New(nil),nil,discardLogger())
	ts:=httptest.NewServer(s.Handler())
	defer ts.Close()
	resp,err:=http.Get(ts.URL+"/api/public/model-costs")
	if err!=nil {t.Fatal(err)}
	defer resp.Body.Close()
	var doc struct{Models []publicModelCosts `json:"models"`}
	if err:=json.NewDecoder(resp.Body).Decode(&doc);err!=nil{t.Fatal(err)}
	if len(doc.Models)!=1 || doc.Models[0].Complete || doc.Models[0].PricedRoutes!=0 || len(doc.Models[0].Rates)!=0 {
		t.Fatalf("unknown price must not be represented as $0: %+v",doc.Models)
	}
}
