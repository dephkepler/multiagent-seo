package pexels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://api.pexels.com/v1"

type Photo struct {
	URL             string
	Photographer    string
	PhotographerURL string
	SourceURL       string
	Alt             string
}

type Client struct {
	apiKey  string
	http    *http.Client
	baseURL string
}

func newClient(apiKey string) *Client {
	return newClientWithBaseURL(apiKey, defaultBaseURL)
}

func newClientWithBaseURL(apiKey, baseURL string) *Client {
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
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pexels search request for query %q: %w", query, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pexels api returned status %d for query %q", resp.StatusCode, query)
	}

	var body searchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode pexels response: %w", err)
	}

	out := make([]Photo, 0, len(body.Photos))
	for _, p := range body.Photos {
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
