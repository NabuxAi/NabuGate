package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FetchRouterahPrices reads Routerah's public catalogue. Rates are in USD per
// million input/output tokens; fixed-price image entries are per generated
// image. Non-image fixed-price units are intentionally left unpriced because
// NabuGate cannot meter their vendor-specific output units yet.
func FetchRouterahPrices(ctx context.Context, catalogURL string) (map[string]Price, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogURL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("routerah catalog returned HTTP %d", resp.StatusCode)
	}
	var catalog struct {
		Models []struct {
			Name        string  `json:"model_name"`
			Type        string  `json:"type"`
			Free        bool    `json:"is_free"`
			Fixed       bool    `json:"is_fixed_price"`
			InputPrice  float64 `json:"input_price"`
			OutputPrice float64 `json:"output_price"`
			FixedPrice  float64 `json:"fixed_price"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&catalog); err != nil {
		return nil, fmt.Errorf("decode routerah catalog: %w", err)
	}
	if len(catalog.Models) == 0 {
		return nil, fmt.Errorf("routerah catalog has no models")
	}
	prices := make(map[string]Price, len(catalog.Models))
	for _, model := range catalog.Models {
		name := strings.TrimSpace(model.Name)
		if name == "" || model.InputPrice < 0 || model.OutputPrice < 0 || model.FixedPrice < 0 {
			continue
		}
		if model.Free || strings.HasSuffix(name, ":free") {
			prices[name] = Price{}
		} else if model.Fixed && model.Type == "image" && model.FixedPrice > 0 {
			prices[name] = Price{PerImage: model.FixedPrice}
		} else if !model.Fixed && (model.InputPrice > 0 || model.OutputPrice > 0) {
			prices[name] = Price{Input: model.InputPrice, Output: model.OutputPrice}
		}
	}
	if len(prices) == 0 {
		return nil, fmt.Errorf("routerah catalog has no meterable prices")
	}
	return prices, nil
}
