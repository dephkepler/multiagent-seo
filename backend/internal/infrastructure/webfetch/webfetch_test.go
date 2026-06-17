package webfetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

const sampleHTML = `<!DOCTYPE html>
<html>
<head>
	<title>  Acme Widgets — Home  </title>
	<meta charset="utf-8">
	<meta name="description" content="  We sell the finest widgets.  ">
	<meta name="keywords" content="widgets, gadgets">
	<style>.x{color:red}</style>
</head>
<body>
	<h1>Welcome to Acme</h1>
	<h2>Our Products</h2>
	<h3>Widgets</h3>
	<h4>Not collected</h4>
	<p>Acme builds reliable widgets for every need.</p>
	<a href="https://example.com/about">About</a>
	<a href="/contact">Contact</a>
	<a href="https://partner.example.org">Partner</a>
	<a>missing href</a>
	<script>var ignore = "do not include";</script>
</body>
</html>`

func TestParsePage(t *testing.T) {
	page, err := parsePage(strings.NewReader(sampleHTML))
	if err != nil {
		t.Fatalf("parsePage: %v", err)
	}

	if page.Title != "Acme Widgets — Home" {
		t.Errorf("Title = %q", page.Title)
	}

	if page.MetaDescription != "We sell the finest widgets." {
		t.Errorf("MetaDescription = %q", page.MetaDescription)
	}

	wantHeadings := []string{"Welcome to Acme", "Our Products", "Widgets"}
	if !slices.Equal(page.Headings, wantHeadings) {
		t.Errorf("Headings = %v, want %v", page.Headings, wantHeadings)
	}

	wantLinks := []string{"https://example.com/about", "/contact", "https://partner.example.org"}
	if !slices.Equal(page.Links, wantLinks) {
		t.Errorf("Links = %v, want %v", page.Links, wantLinks)
	}

	if !strings.Contains(page.TextSample, "Welcome to Acme") {
		t.Errorf("TextSample missing heading text: %q", page.TextSample)
	}
	if !strings.Contains(page.TextSample, "reliable widgets") {
		t.Errorf("TextSample missing paragraph text: %q", page.TextSample)
	}
	if strings.Contains(page.TextSample, "do not include") {
		t.Errorf("TextSample leaked <script> text: %q", page.TextSample)
	}
}

func TestParsePageTextSampleCap(t *testing.T) {
	var b strings.Builder
	b.WriteString("<html><body>")
	for i := 0; i < 5000; i++ {
		b.WriteString("<p>word</p>")
	}
	b.WriteString("</body></html>")

	page, err := parsePage(strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("parsePage: %v", err)
	}
	if len(page.TextSample) > maxTextSample {
		t.Errorf("TextSample length = %d, want <= %d", len(page.TextSample), maxTextSample)
	}
}

func TestTruncateRunesBoundary(t *testing.T) {
	s := strings.Repeat("é", 50)
	out := truncateRunes(s, 51)
	if !utf8.ValidString(out) {
		t.Fatalf("truncateRunes produced invalid UTF-8: %q", out)
	}
	if len(out) > 51 {
		t.Errorf("len(out) = %d, want <= 51", len(out))
	}
	if got := truncateRunes("hello", 100); got != "hello" {
		t.Errorf("truncateRunes(short) = %q", got)
	}
}

func testFetcher() *Fetcher {
	return newFetcher(nil, true)
}

func TestFetchRedirectToBlockedTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer srv.Close()

	if _, err := testFetcher().Fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("expected error fetching redirect to blocked target, got nil")
	}
}

func TestFetchOversizedBodyCapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>start</p>"))
		junk := strings.Repeat("x", 4<<20)
		_, _ = w.Write([]byte("<p>" + junk + "</p></body></html>"))
	}))
	defer srv.Close()

	page, err := testFetcher().Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch oversized body: %v", err)
	}
	if len(page.TextSample) > maxTextSample {
		t.Errorf("TextSample length = %d, want <= %d", len(page.TextSample), maxTextSample)
	}
}

func TestFetchNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := testFetcher().Fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("expected error for non-2xx status, got nil")
	}
}

func TestFetchUnsupportedContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 not html"))
	}))
	defer srv.Close()

	_, err := testFetcher().Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for application/pdf content type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported content type") {
		t.Errorf("error = %v, want unsupported content type", err)
	}
}

func TestFetchUnsupportedScheme(t *testing.T) {
	if _, err := testFetcher().Fetch(context.Background(), "file:///etc/passwd"); err == nil {
		t.Fatal("expected error for file:// scheme, got nil")
	}
}

func TestFetchTextHTMLAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(sampleHTML))
	}))
	defer srv.Close()

	page, err := testFetcher().Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch text/html: %v", err)
	}
	if page.Title != "Acme Widgets — Home" {
		t.Errorf("Title = %q", page.Title)
	}
}
