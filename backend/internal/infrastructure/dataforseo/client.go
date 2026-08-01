package dataforseo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
				Type         string          `json:"type"`
				RankAbsolute int             `json:"rank_absolute"`
				URL          string          `json:"url"`
				Title        string          `json:"title"`
				Description  string          `json:"description"`
				Items        json.RawMessage `json:"items"`
			} `json:"items"`
		} `json:"result"`
	} `json:"tasks"`
}

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

// parseSubItems distinguishes "no nested data" (empty raw) from "parse failed":
// the latter is logged so a malformed PAA/featured-snippet section is not
// silently returned as empty. parseSERPResponse tolerates the dropped items,
// so DEBUG is the right level.
func parseSubItems(raw json.RawMessage) []serpSubItem {
	if len(raw) == 0 {
		return nil
	}
	var out []serpSubItem
	if err := json.Unmarshal(raw, &out); err != nil {
		slog.Default().Debug("dataforseo: failed to parse serp subitems", "err", err)
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
		LocationCode: 2840,
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
		return nil, fmt.Errorf("dataforseo request for keyword %q: %w", keyword, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dataforseo api returned status %d for keyword %q", resp.StatusCode, keyword)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read dataforseo response: %w", err)
	}
	return parseSERPResponse(body, keyword, limit)
}

func parseSERPResponse(body []byte, keyword string, limit int) (*articles.CompetitorData, error) {
	var result serpResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode dataforseo response: %w", err)
	}

	out := &articles.CompetitorData{
		Keyword:  keyword,
		SerpDate: time.Now().Format("2006-01-02"),
		Results:  []articles.SERPItem{},
	}

	if len(result.Tasks) == 0 || len(result.Tasks[0].Result) == 0 {
		return out, nil
	}

	for _, item := range result.Tasks[0].Result[0].Items {
		switch item.Type {
		case "organic":
			if len(out.Results) >= limit {
				continue
			}
			out.Results = append(out.Results, articles.SERPItem{
				Rank:        item.RankAbsolute,
				URL:         item.URL,
				Title:       item.Title,
				Description: item.Description,
			})
		case "people_also_ask":
			for _, sub := range parseSubItems(item.Items) {
				if sub.Title == "" {
					continue
				}
				out.PAA = append(out.PAA, sub.Title)
			}
		case "featured_snippet", "answer_box":
			if out.FeaturedSnippet != nil {
				continue
			}
			for _, sub := range parseSubItems(item.Items) {
				if sub.Title == "" {
					continue
				}
				out.FeaturedSnippet = &articles.FeaturedSnippet{
					Title:       sub.Title,
					Description: sub.Description,
				}
				break
			}
		}
	}

	return out, nil
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
