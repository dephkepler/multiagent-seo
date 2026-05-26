package publisher

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubResolver returns queue[i] on the i-th call so tests can assert
// the order in which placeholders are resolved.
type stubResolver struct {
	queue []stubResult
	calls int
}

type stubResult struct {
	url string
	err error
}

func (s *stubResolver) Resolve(_ context.Context, _, _, _ string) (ResolvedImage, error) {
	if s.calls >= len(s.queue) {
		s.calls++
		return ResolvedImage{}, errors.New("stub: out of results")
	}
	r := s.queue[s.calls]
	s.calls++
	return ResolvedImage{URL: r.url}, r.err
}

type fixedResolver struct{ url string }

func (f fixedResolver) Resolve(_ context.Context, _, _, _ string) (ResolvedImage, error) {
	return ResolvedImage{URL: f.url}, nil
}

type errResolver struct{}

func (errResolver) Resolve(_ context.Context, _, _, _ string) (ResolvedImage, error) {
	return ResolvedImage{}, errors.New("boom")
}

func TestRenderHTML(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		resolver    ImageResolver
		mustContain []string
		mustNotHave []string
	}{
		{
			name:        "heading",
			input:       "## Heading",
			mustContain: []string{"<h2"},
		},
		{
			name:        "table",
			input:       "| H1 | H2 |\n| --- | --- |\n| a | b |\n",
			mustContain: []string{"<table>", "<td>"},
		},
		{
			name:        "img placeholder stripped when resolver is nil",
			input:       "Before [IMG | desc | ALT: alt text | 1200px horizontal AI-generated] after",
			resolver:    nil,
			mustNotHave: []string{"[IMG", "<img"},
		},
		{
			name:        "internal link placeholder stripped",
			input:       "Before [INTERNAL_LINK | anchor: read more | related topic] after",
			mustNotHave: []string{"[INTERNAL_LINK"},
		},
		{
			name:        "html passthrough",
			input:       `Visit <a href="https://example.com" rel="nofollow" target="_blank">site</a>`,
			mustContain: []string{`rel="nofollow"`, `<a href="https://example.com" rel="nofollow" target="_blank">site</a>`},
		},
		{
			name:     "img placeholder resolved to <img>",
			input:    "Intro paragraph.\n\n[IMG | a happy golden retriever | ALT: something | 1200px horizontal AI-generated]\n\nMore text.",
			resolver: fixedResolver{url: "https://example.com/photo.jpg"},
			mustContain: []string{
				`<img src="https://example.com/photo.jpg" alt="something" loading="lazy"`,
			},
			mustNotHave: []string{"[IMG"},
		},
		{
			name:        "img placeholder stripped on resolver error",
			input:       "Before [IMG | failing query | ALT: alt | meta] after",
			resolver:    errResolver{},
			mustContain: []string{"Before", "after"},
			mustNotHave: []string{"[IMG", "<img"},
		},
		{
			name:  "mixed: first resolves, second errors",
			input: "One [IMG | first | ALT: a1 | meta] two [IMG | second | ALT: a2 | meta] three",
			resolver: &stubResolver{queue: []stubResult{
				{url: "https://cdn.example/one.jpg"},
				{err: errors.New("nope")},
			}},
			mustContain: []string{
				`<img src="https://cdn.example/one.jpg" alt="a1" loading="lazy"`,
				"three",
			},
			mustNotHave: []string{"[IMG", "alt=\"a2\""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := RenderHTML(context.Background(), tc.input, RenderOptions{
				Resolver:    tc.resolver,
				Attribution: true,
			})
			for _, want := range tc.mustContain {
				if !strings.Contains(got, want) {
					t.Errorf("expected output to contain %q, got: %s", want, got)
				}
			}
			for _, bad := range tc.mustNotHave {
				if strings.Contains(got, bad) {
					t.Errorf("expected output NOT to contain %q, got: %s", bad, got)
				}
			}
		})
	}
}

func TestRenderHTMLAttributionOff(t *testing.T) {
	type photoResolver struct{}
	// inline resolver returning full Pexels metadata
	resolver := imgRes(ResolvedImage{
		URL:             "https://example.com/p.jpg",
		Photographer:    "Jane Doe",
		PhotographerURL: "https://example.com/jane",
		SourceURL:       "https://example.com/source",
	})
	_ = photoResolver{}

	on, _ := RenderHTML(context.Background(), "[IMG | x | ALT: a]", RenderOptions{Resolver: resolver, Attribution: true})
	off, _ := RenderHTML(context.Background(), "[IMG | x | ALT: a]", RenderOptions{Resolver: resolver, Attribution: false})

	if !strings.Contains(on, "<figcaption>") {
		t.Errorf("attribution=true must include <figcaption>; got %s", on)
	}
	if strings.Contains(off, "<figcaption>") || strings.Contains(off, "<figure>") {
		t.Errorf("attribution=false must omit <figcaption>/<figure>; got %s", off)
	}
}

func TestRenderHTMLStats(t *testing.T) {
	stub := &stubResolver{queue: []stubResult{
		{url: "https://cdn.example/one.jpg"},
		{err: errors.New("nope")},
	}}
	_, stats := RenderHTML(context.Background(), "[IMG | a | ALT: a1] mid [IMG | b | ALT: a2] end", RenderOptions{
		Resolver:    stub,
		Attribution: true,
	})
	if stats.ImagesRequested != 2 {
		t.Errorf("ImagesRequested = %d, want 2", stats.ImagesRequested)
	}
	if stats.ImagesResolved != 1 {
		t.Errorf("ImagesResolved = %d, want 1", stats.ImagesResolved)
	}
	if stats.ImagesSkipped != 1 {
		t.Errorf("ImagesSkipped = %d, want 1", stats.ImagesSkipped)
	}

	_, stats = RenderHTML(context.Background(), "[IMG | a | ALT: a1]", RenderOptions{Resolver: nil})
	if stats.ImagesRequested != 1 || stats.ImagesResolved != 0 || stats.ImagesSkipped != 1 {
		t.Errorf("nil resolver stats = %+v, want {1,0,1}", stats)
	}
}

// imgRes returns a resolver that always answers with the given image.
type imgRes ResolvedImage

func (r imgRes) Resolve(_ context.Context, _, _, _ string) (ResolvedImage, error) {
	return ResolvedImage(r), nil
}
