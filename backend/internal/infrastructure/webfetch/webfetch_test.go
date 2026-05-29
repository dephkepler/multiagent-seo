package webfetch

import (
	"slices"
	"strings"
	"testing"
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
