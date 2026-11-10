package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nabugate/internal/usage"
)

func fetchRouterahRates(catalogURL string) (map[string]usage.Price, error) {
	if strings.TrimSpace(catalogURL) == "" {
		return nil, fmt.Errorf("models_url is missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return usage.FetchRouterahPrices(ctx, catalogURL)
}
