// Package pexels is a minimal client for the Pexels stock-photo search API.
package pexels

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://api.pexels.com/v1"

type Photo struct {
	URL             string
	Photographer    string
	PhotographerURL string
	SourceURL       string // Pexels page URL, used for attribution
	Alt             string // Pexels-provided alt text — used by callers for relevance scoring
}

type Client struct {
	apiKey  string
	http    *http.Client
	baseURL string
}

func New(apiKey string) *Client {
	return NewWithBaseURL(apiKey, defaultBaseURL)
}

// NewWithBaseURL is the test seam letting httptest.Server stand in for Pexels.
func NewWithBaseURL(apiKey, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 10 * time.Second},
		baseURL: baseURL,
	}
}

type searchResponse struct {
	Photos []struct {
		URL string `json:"url"`
		Src struct {
			Landscape string `json:"landscape"`
			Large     string `json:"large"`
			Large2x   string `json:"large2x"`
		} `json:"src"`
		Photographer    string `json:"photographer"`
		PhotographerURL string `json:"photographer_url"`
		Alt             string `json:"alt"`
	} `json:"photos"`
}

// Search returns the first landscape photo for query; thin wrapper around
// SearchN(ctx, query, 1) kept so existing callers and tests don't churn.
func (c *Client) Search(ctx context.Context, query string) (*Photo, error) {
	photos, err := c.SearchN(ctx, query, 1)
	if err != nil {
		return nil, err
	}
	if len(photos) == 0 {
		return nil, fmt.Errorf("pexels: no photos for query %q", query)
	}
	return &photos[0], nil
}

// SearchN returns up to n landscape candidates for query so callers can
// score them locally (e.g. reject photos whose alt text shares no tokens
// with the article keyword). Photos with no usable src URL are dropped.
func (c *Client) SearchN(ctx context.Context, query string, n int) ([]Photo, error) {
	if n < 1 {
		n = 1
	}
	u := fmt.Sprintf("%s/search?query=%s&orientation=landscape&per_page=%d",
		c.baseURL, url.QueryEscape(query), n)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("pexels new request: %w", err)
	}
	// Pexels expects the bare key in Authorization (no "Bearer ").
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pexels request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pexels status %d", resp.StatusCode)
	}

	var body searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode pexels response: %w", err)
	}

	out := make([]Photo, 0, len(body.Photos))
	for _, p := range body.Photos {
		// Prefer "landscape" (blog-friendly aspect); some photos omit it.
		imgURL := p.Src.Landscape
		if imgURL == "" {
			imgURL = p.Src.Large
		}
		if imgURL == "" {
			continue
		}
		out = append(out, Photo{
			URL:             imgURL,
			Photographer:    p.Photographer,
			PhotographerURL: p.PhotographerURL,
			SourceURL:       p.URL,
			Alt:             p.Alt,
		})
	}
	return out, nil
}
