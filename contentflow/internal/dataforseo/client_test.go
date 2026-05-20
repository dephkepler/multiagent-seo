package dataforseo

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const mixedFixture = `{
  "tasks": [{
    "result": [{
      "items": [
        {"type": "featured_snippet", "rank_absolute": 1, "title": "Best widgets 2026", "description": "Widgets are devices that..."},
        {"type": "organic", "rank_absolute": 2, "url": "https://a.example/1", "title": "Top 10 widgets", "description": "A list of widgets."},
        {"type": "people_also_ask", "rank_absolute": 3, "items": [
          {"type": "people_also_ask_element", "title": "What is a widget?", "description": "A widget is..."},
          {"type": "people_also_ask_element", "title": "How do widgets work?", "description": "They work by..."},
          {"type": "people_also_ask_element", "title": "Are widgets safe?", "description": "Yes."}
        ]},
        {"type": "organic", "rank_absolute": 4, "url": "https://b.example/2", "title": "Widget guide", "description": "Learn widgets."}
      ]
    }]
  }]
}`

const noExtrasFixture = `{
  "tasks": [{
    "result": [{
      "items": [
        {"type": "organic", "rank_absolute": 1, "url": "https://a.example/1", "title": "Just organic 1", "description": "d1"},
        {"type": "organic", "rank_absolute": 2, "url": "https://a.example/2", "title": "Just organic 2", "description": "d2"}
      ]
    }]
  }]
}`

func newTestClient(t *testing.T, body string) (*RealClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
		if auth != expected {
			t.Errorf("missing/incorrect basic auth header: %q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))

	c := New("user", "pass")
	c.http.Transport = rewriteTransport{target: srv.URL}
	return c, srv
}

// rewriteTransport redirects outgoing requests to the test server while
// preserving path and query.
type rewriteTransport struct {
	target string
}

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	newURL := r.target + req.URL.Path
	if req.URL.RawQuery != "" {
		newURL += "?" + req.URL.RawQuery
	}
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header
	return http.DefaultTransport.RoundTrip(newReq)
}

func TestGetSERP_ParsesPAAAndFeaturedSnippet(t *testing.T) {
	c, srv := newTestClient(t, mixedFixture)
	defer srv.Close()

	data, err := c.GetSERP(context.Background(), "widgets", "en", 10)
	if err != nil {
		t.Fatalf("GetSERP returned error: %v", err)
	}

	if len(data.Results) != 2 {
		t.Errorf("organic results: want 2, got %d", len(data.Results))
	}

	wantPAA := []string{"What is a widget?", "How do widgets work?", "Are widgets safe?"}
	if len(data.PAA) != len(wantPAA) {
		t.Fatalf("PAA length: want %d, got %d (%v)", len(wantPAA), len(data.PAA), data.PAA)
	}
	for i, q := range wantPAA {
		if data.PAA[i] != q {
			t.Errorf("PAA[%d]: want %q, got %q", i, q, data.PAA[i])
		}
	}

	if data.FeaturedSnippet == nil {
		t.Fatal("FeaturedSnippet: want populated, got nil")
	}
	if data.FeaturedSnippet.Title != "Best widgets 2026" {
		t.Errorf("FeaturedSnippet.Title: got %q", data.FeaturedSnippet.Title)
	}
	if !strings.Contains(data.FeaturedSnippet.Description, "Widgets are devices") {
		t.Errorf("FeaturedSnippet.Description: got %q", data.FeaturedSnippet.Description)
	}
}

func TestGetSERP_NoExtras(t *testing.T) {
	c, srv := newTestClient(t, noExtrasFixture)
	defer srv.Close()

	data, err := c.GetSERP(context.Background(), "widgets", "en", 10)
	if err != nil {
		t.Fatalf("GetSERP returned error: %v", err)
	}

	if len(data.Results) != 2 {
		t.Errorf("organic results: want 2, got %d", len(data.Results))
	}
	if len(data.PAA) != 0 {
		t.Errorf("PAA: want empty, got %v", data.PAA)
	}
	if data.FeaturedSnippet != nil {
		t.Errorf("FeaturedSnippet: want nil, got %+v", data.FeaturedSnippet)
	}
}
