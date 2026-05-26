package pexels

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const okFixture = `{
  "photos": [{
    "url": "https://www.pexels.com/photo/cat-12345/",
    "src": {
      "landscape": "https://images.pexels.com/photos/12345/landscape.jpg",
      "large":     "https://images.pexels.com/photos/12345/large.jpg",
      "large2x":   "https://images.pexels.com/photos/12345/large2x.jpg"
    },
    "photographer": "Jane Doe",
    "photographer_url": "https://www.pexels.com/@janedoe"
  }]
}`

const emptyFixture = `{"photos": []}`

func TestSearch(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantErr    bool
		wantURL    string
		wantAuthor string
	}{
		{
			name:       "ok",
			status:     http.StatusOK,
			body:       okFixture,
			wantURL:    "https://images.pexels.com/photos/12345/landscape.jpg",
			wantAuthor: "Jane Doe",
		},
		{
			name:    "no photos",
			status:  http.StatusOK,
			body:    emptyFixture,
			wantErr: true,
		},
		{
			name:    "non-200",
			status:  http.StatusUnauthorized,
			body:    `{"error":"unauthorized"}`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "test-key" {
					t.Errorf("Authorization header: want %q, got %q", "test-key", got)
				}
				if r.URL.Path != "/search" {
					t.Errorf("path: want /search, got %q", r.URL.Path)
				}
				q := r.URL.Query()
				if q.Get("orientation") != "landscape" {
					t.Errorf("orientation: want landscape, got %q", q.Get("orientation"))
				}
				if q.Get("per_page") != "1" {
					t.Errorf("per_page: want 1, got %q", q.Get("per_page"))
				}
				if q.Get("query") == "" {
					t.Errorf("query: want non-empty")
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			c := NewWithBaseURL("test-key", srv.URL)
			got, err := c.Search(context.Background(), "happy cats")

			if tc.wantErr {
				if err == nil {
					t.Fatalf("Search: want error, got nil (photo=%+v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Search: unexpected error: %v", err)
			}
			if got.URL != tc.wantURL {
				t.Errorf("URL: want %q, got %q", tc.wantURL, got.URL)
			}
			if got.Photographer != tc.wantAuthor {
				t.Errorf("Photographer: want %q, got %q", tc.wantAuthor, got.Photographer)
			}
			if !strings.Contains(got.SourceURL, "pexels.com") {
				t.Errorf("SourceURL: want pexels.com URL, got %q", got.SourceURL)
			}
		})
	}
}

func TestSearch_FallsBackToLargeWhenLandscapeMissing(t *testing.T) {
	body := `{
	  "photos": [{
	    "url": "https://www.pexels.com/photo/x/",
	    "src": { "large": "https://images.pexels.com/photos/x/large.jpg" },
	    "photographer": "Anon",
	    "photographer_url": ""
	  }]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	c := NewWithBaseURL("k", srv.URL)
	got, err := c.Search(context.Background(), "anything")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got.URL != "https://images.pexels.com/photos/x/large.jpg" {
		t.Errorf("URL fallback: got %q", got.URL)
	}
}
