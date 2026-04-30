package dataforseo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const apiBase = "https://api.dataforseo.com/v3"

type SERPItem struct {
	Rank        int    `json:"rank"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type CompetitorData struct {
	Keyword  string     `json:"keyword"`
	SerpDate string     `json:"serp_date"`
	Results  []SERPItem `json:"results"`
}

type Client interface {
	GetSERP(ctx context.Context, keyword, languageCode string, limit int) (*CompetitorData, error)
}

// RealClient calls the DataForSEO SERP API with Basic Auth.
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
			} `json:"items"`
		} `json:"result"`
	} `json:"tasks"`
}

func (c *RealClient) GetSERP(ctx context.Context, keyword, languageCode string, limit int) (*CompetitorData, error) {
	if languageCode == "" {
		languageCode = "en"
	}

	payload, err := json.Marshal([]serpRequest{{
		Keyword:      keyword,
		LanguageCode: languageCode,
		LocationCode: 2840, // United States; override via config when needed
		Device:       "desktop",
		Depth:        limit,
	}})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiBase+"/serp/google/organic/live/advanced",
		bytes.NewReader(payload))
	if err != nil {
		return nil, err
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

	var result serpResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode dataforseo response: %w", err)
	}

	data := &CompetitorData{
		Keyword:  keyword,
		SerpDate: time.Now().Format("2006-01-02"),
		Results:  []SERPItem{},
	}

	if len(result.Tasks) == 0 || len(result.Tasks[0].Result) == 0 {
		return data, nil
	}

	for _, item := range result.Tasks[0].Result[0].Items {
		if item.Type != "organic" {
			continue
		}
		if len(data.Results) >= limit {
			break
		}
		data.Results = append(data.Results, SERPItem{
			Rank:        item.RankAbsolute,
			URL:         item.URL,
			Title:       item.Title,
			Description: item.Description,
		})
	}

	return data, nil
}

// MockClient returns hardcoded SERP data — used when credentials are not configured.
type MockClient struct{}

func NewMock() *MockClient { return &MockClient{} }

func (m *MockClient) GetSERP(_ context.Context, keyword, _ string, limit int) (*CompetitorData, error) {
	items := []SERPItem{
		{Rank: 1, URL: "https://example.com/1", Title: "Complete guide: " + keyword, Description: "Comprehensive guide about " + keyword + " with expert tips."},
		{Rank: 2, URL: "https://example.com/2", Title: keyword + " — full overview 2026", Description: "Everything you need to know about " + keyword + "."},
		{Rank: 3, URL: "https://example.com/3", Title: "How to " + keyword + " step by step", Description: "Step by step instructions for " + keyword + "."},
		{Rank: 4, URL: "https://example.com/4", Title: keyword + ": tips and mistakes to avoid", Description: "Expert tips and common mistakes about " + keyword + "."},
		{Rank: 5, URL: "https://example.com/5", Title: "Ultimate " + keyword + " resource", Description: "The ultimate resource for understanding " + keyword + "."},
	}
	if limit < len(items) {
		items = items[:limit]
	}
	return &CompetitorData{
		Keyword:  keyword,
		SerpDate: time.Now().Format("2006-01-02"),
		Results:  items,
	}, nil
}
