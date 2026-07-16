// Package photos proxies stock-photo search (Pexels) through the gateway, so
// projects use one NabuGate key for photos too and the Pexels API key stays a
// gateway secret — same pattern as the LLM providers.
package photos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DefaultBaseURL is the Pexels REST API root.
const DefaultBaseURL = "https://api.pexels.com"

// Src carries the pre-sized variants Pexels serves per photo.
type Src struct {
	Original  string `json:"original"`
	Large2x   string `json:"large2x"`
	Large     string `json:"large"`
	Medium    string `json:"medium"`
	Small     string `json:"small"`
	Portrait  string `json:"portrait"`
	Landscape string `json:"landscape"`
	Tiny      string `json:"tiny"`
}

// Photo is the normalized subset of a Pexels photo the gateway exposes.
type Photo struct {
	ID              int64  `json:"id"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	URL             string `json:"url"`
	Photographer    string `json:"photographer"`
	PhotographerURL string `json:"photographer_url"`
	AvgColor        string `json:"avg_color"`
	Alt             string `json:"alt"`
	Src             Src    `json:"src"`
}

// SearchResult is the normalized search response.
type SearchResult struct {
	Photos       []Photo `json:"photos"`
	Page         int     `json:"page"`
	PerPage      int     `json:"per_page"`
	TotalResults int     `json:"total_results"`
	Provider     string  `json:"provider"`
}

// SearchParams are the supported search filters. An empty Query switches to
// Pexels' curated feed (a hand-picked default gallery).
type SearchParams struct {
	Query       string
	Orientation string // landscape | portrait | square | ""
	Size        string // large | medium | small | "" (search only)
	PerPage     int    // 1..80, default 10
	Page        int    // >= 1
	Locale      string // e.g. "en-US"
}

// Client is a minimal Pexels API client.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// New returns a Client, or nil when apiKey is empty (feature disabled).
func New(apiKey, baseURL string) *Client {
	if apiKey == "" {
		return nil
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Search runs a photo search and returns the normalized result. With an empty
// query it serves Pexels' curated feed instead, so galleries have a default.
func (c *Client) Search(ctx context.Context, p SearchParams) (*SearchResult, error) {
	if p.PerPage <= 0 {
		p.PerPage = 10
	}
	if p.PerPage > 80 {
		p.PerPage = 80
	}
	if p.Page <= 0 {
		p.Page = 1
	}

	q := url.Values{}
	q.Set("per_page", strconv.Itoa(p.PerPage))
	q.Set("page", strconv.Itoa(p.Page))
	endpoint := "/v1/curated?"
	if p.Query != "" {
		endpoint = "/v1/search?"
		q.Set("query", p.Query)
		switch p.Orientation {
		case "landscape", "portrait", "square":
			q.Set("orientation", p.Orientation)
		}
		switch p.Size {
		case "large", "medium", "small":
			q.Set("size", p.Size)
		}
		if p.Locale != "" {
			q.Set("locale", p.Locale)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+endpoint+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pexels request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("pexels response read failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pexels returned status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var out SearchResult
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("pexels response parse failed: %w", err)
	}
	out.Provider = "pexels"
	return &out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
