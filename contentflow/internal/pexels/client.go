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
	} `json:"photos"`
}

// Search returns the first landscape photo for query, or an error when Pexels
// has zero matches so callers can decide whether to strip or retry.
func (c *Client) Search(ctx context.Context, query string) (*Photo, error) {
	u := fmt.Sprintf("%s/search?query=%s&orientation=landscape&per_page=1",
		c.baseURL, url.QueryEscape(query))

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

	if len(body.Photos) == 0 {
		return nil, fmt.Errorf("pexels: no photos for query %q", query)
	}

	p := body.Photos[0]
	// Prefer "landscape" (blog-friendly aspect); some photos omit it.
	imgURL := p.Src.Landscape
	if imgURL == "" {
		imgURL = p.Src.Large
	}
	if imgURL == "" {
		return nil, fmt.Errorf("pexels: photo has no usable src for query %q", query)
	}

	return &Photo{
		URL:             imgURL,
		Photographer:    p.Photographer,
		PhotographerURL: p.PhotographerURL,
		SourceURL:       p.URL,
	}, nil
}
