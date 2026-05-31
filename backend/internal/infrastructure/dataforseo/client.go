// Package dataforseo adapts the DataForSEO SERP API to the articles.SERPProvider
// port, mapping the response onto domain value types.
package dataforseo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"multiagent-seo/internal/domain/articles"
)

const apiBase = "https://api.dataforseo.com/v3"

type RealClient struct {
	login    string
	password string
	http     *http.Client
}

func New(login, password string) *RealClient {
	return &RealClient{
		login:    login,
		password: password,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

type serpRequest struct {
	Keyword      string `json:"keyword"`
	LanguageCode string `json:"language_code"`
	LocationCode int    `json:"location_code"`
	Device       string `json:"device"`
	Depth        int    `json:"depth"`
}

type serpResponse struct {
	Tasks []struct {
		Result []struct {
			Items []struct {
				Type         string `json:"type"`
				RankAbsolute int    `json:"rank_absolute"`
				URL          string `json:"url"`
				Title        string `json:"title"`
				Description  string `json:"description"`
				// Aggregate blocks (people_also_ask, featured_snippet) nest their
				// text here. DataForSEO is inconsistent about the shape — objects in
				// some blocks, bare strings in others (e.g. related_searches) — so
				// keep it raw and decode tolerantly rather than failing the whole
				// response when one block's shape is unexpected.
				Items json.RawMessage `json:"items"`
			} `json:"items"`
		} `json:"result"`
	} `json:"tasks"`
}

// serpSubItem is one entry inside an aggregate block's nested items. Some blocks
// list objects, others list bare strings — UnmarshalJSON accepts both, treating
// a string as the title.
type serpSubItem struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func (s *serpSubItem) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &s.Title)
	}
	type alias serpSubItem
	return json.Unmarshal(b, (*alias)(s))
}

// subItems decodes a block's nested items, tolerating shapes we don't expect by
// returning nil rather than failing the whole SERP parse.
func subItems(raw json.RawMessage) []serpSubItem {
	if len(raw) == 0 {
		return nil
	}
	var out []serpSubItem
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func (c *RealClient) GetSERP(ctx context.Context, keyword, languageCode string, limit int) (*articles.CompetitorData, error) {
	if languageCode == "" {
		languageCode = "en"
	}

	payload, err := json.Marshal([]serpRequest{{
		Keyword:      keyword,
		LanguageCode: languageCode,
		LocationCode: 2840, // United States
		Device:       "desktop",
		Depth:        limit,
	}})
	if err != nil {
		return nil, fmt.Errorf("dataforseo marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiBase+"/serp/google/organic/live/advanced",
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("dataforseo build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.login, c.password)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dataforseo request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dataforseo status %d", resp.StatusCode)
	}

	// Cap the body so a hostile/huge response can't be read fully into memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read dataforseo response: %w", err)
	}
	return parseSERPResponse(body, keyword, limit)
}

// parseSERPResponse decodes a SERP payload and maps it onto CompetitorData.
// Split out from GetSERP so the (DataForSEO-shape-sensitive) parsing is unit
// testable without an HTTP round-trip.
func parseSERPResponse(body []byte, keyword string, limit int) (*articles.CompetitorData, error) {
	var result serpResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode dataforseo response: %w", err)
	}

	data := &articles.CompetitorData{
		Keyword:  keyword,
		SerpDate: time.Now().Format("2006-01-02"),
		Results:  []articles.SERPItem{},
	}

	if len(result.Tasks) == 0 || len(result.Tasks[0].Result) == 0 {
		return data, nil
	}

	for _, item := range result.Tasks[0].Result[0].Items {
		switch item.Type {
		case "organic":
			if len(data.Results) >= limit {
				continue
			}
			data.Results = append(data.Results, articles.SERPItem{
				Rank:        item.RankAbsolute,
				URL:         item.URL,
				Title:       item.Title,
				Description: item.Description,
			})
		case "people_also_ask":
			for _, sub := range subItems(item.Items) {
				if sub.Title == "" {
					continue
				}
				data.PAA = append(data.PAA, sub.Title)
			}
		case "featured_snippet", "answer_box":
			// DataForSEO nests the snippet text in a child item, not on the
			// parent block; take the first child that carries a title.
			if data.FeaturedSnippet != nil {
				continue
			}
			for _, sub := range subItems(item.Items) {
				if sub.Title == "" {
					continue
				}
				data.FeaturedSnippet = &articles.FeaturedSnippet{
					Title:       sub.Title,
					Description: sub.Description,
				}
				break
			}
		}
	}

	return data, nil
}

type MockClient struct{}

func NewMock() *MockClient { return &MockClient{} }

func (m *MockClient) GetSERP(_ context.Context, keyword, _ string, limit int) (*articles.CompetitorData, error) {
	items := []articles.SERPItem{
		{Rank: 1, URL: "https://example.com/1", Title: "Complete guide: " + keyword, Description: "Comprehensive guide about " + keyword + " with expert tips."},
		{Rank: 2, URL: "https://example.com/2", Title: keyword + " — full overview 2026", Description: "Everything you need to know about " + keyword + "."},
		{Rank: 3, URL: "https://example.com/3", Title: "How to " + keyword + " step by step", Description: "Step by step instructions for " + keyword + "."},
		{Rank: 4, URL: "https://example.com/4", Title: keyword + ": tips and mistakes to avoid", Description: "Expert tips and common mistakes about " + keyword + "."},
		{Rank: 5, URL: "https://example.com/5", Title: "Ultimate " + keyword + " resource", Description: "The ultimate resource for understanding " + keyword + "."},
	}
	if limit < len(items) {
		items = items[:limit]
	}
	return &articles.CompetitorData{
		Keyword:  keyword,
		SerpDate: time.Now().Format("2006-01-02"),
		Results:  items,
	}, nil
}
