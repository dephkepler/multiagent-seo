package linkbuilding_test

import (
	"context"
	"errors"
	"testing"

	applb "multiagent-seo/internal/application/linkbuilding"
	domain "multiagent-seo/internal/domain/linkbuilding"
	"multiagent-seo/pkg/jobrunner"
)

type fakeSource struct {
	sites   []domain.Website
	written []domain.Result
}

func (f *fakeSource) List(context.Context, string) ([]domain.Website, error) { return f.sites, nil }

// WriteResults appends so chunked flushes (and out-of-order parallel results)
// are all captured.
func (f *fakeSource) WriteResults(_ context.Context, _ string, r []domain.Result) error {
	f.written = append(f.written, r...)
	return nil
}

type fakeFetcher struct{ page domain.Page }

func (f fakeFetcher) Fetch(context.Context, string) (domain.Page, error) { return f.page, nil }

// scriptedFetcher returns a per-URL page or error, for partial-failure and
// cancellation tests.
type scriptedFetcher struct {
	pages map[string]domain.Page
	errs  map[string]error
}

func (f scriptedFetcher) Fetch(_ context.Context, url string) (domain.Page, error) {
	if err := f.errs[url]; err != nil {
		return domain.Page{}, err
	}
	return f.pages[url], nil
}

type fakeClassifier struct{ topic string }

func (f fakeClassifier) Classify(context.Context, domain.Page, []string) (string, error) {
	return f.topic, nil
}

func TestQualifyWebsites_WritesPerSiteResults(t *testing.T) {
	src := &fakeSource{sites: []domain.Website{{Row: 2, URL: "https://acme.com"}}}
	fetcher := fakeFetcher{page: domain.Page{Links: []string{
		"https://other.com/a", "https://acme.com/internal", "https://second.net",
	}}}
	svc := applb.NewService(src, fetcher, fakeClassifier{topic: "gambling"}, jobrunner.NewSyncRunner(), nil)

	res, err := svc.QualifyWebsites(context.Background(), applb.QualifyRequest{
		Sheet:          "WEBSITES",
		AcceptedTopics: []string{"gambling", "news"},
	})
	if err != nil {
		t.Fatalf("QualifyWebsites: %v", err)
	}
	if res.WebsitesQueued != 1 {
		t.Fatalf("queued = %d, want 1", res.WebsitesQueued)
	}

	// SyncRunner ran inline, so results are already written back.
	if len(src.written) != 1 {
		t.Fatalf("written = %d, want 1", len(src.written))
	}
	got := src.written[0]
	if got.Row != 2 || got.Topic != "gambling" || !got.Suitable {
		t.Errorf("result = %+v, want row 2 / gambling / suitable", got)
	}
	if got.OutboundDomains != 2 { // other.com, second.net (acme.com is internal)
		t.Errorf("outbound = %d, want 2", got.OutboundDomains)
	}
}

func TestQualifyWebsites_UnacceptedTopicNotSuitable(t *testing.T) {
	src := &fakeSource{sites: []domain.Website{{Row: 2, URL: "https://tech.io"}}}
	svc := applb.NewService(src, fakeFetcher{}, fakeClassifier{topic: "tech"}, jobrunner.NewSyncRunner(), nil)

	if _, err := svc.QualifyWebsites(context.Background(), applb.QualifyRequest{
		Sheet:          "WEBSITES",
		AcceptedTopics: []string{"gambling"},
	}); err != nil {
		t.Fatalf("QualifyWebsites: %v", err)
	}
	if src.written[0].Suitable {
		t.Error("topic not in accepted set must be unsuitable")
	}
}

func TestQualifyWebsites_ParallelPartialFailure(t *testing.T) {
	sites := []domain.Website{
		{Row: 2, URL: "https://a.com"},
		{Row: 3, URL: "https://b.com"}, // fetch fails → unsuitable, run continues
		{Row: 4, URL: "https://c.com"},
	}
	src := &fakeSource{sites: sites}
	fetcher := scriptedFetcher{
		pages: map[string]domain.Page{
			"https://a.com": {Links: []string{"https://x.com"}},
			"https://c.com": {Links: []string{"https://y.com"}},
		},
		errs: map[string]error{"https://b.com": errors.New("boom")},
	}
	svc := applb.NewService(src, fetcher, fakeClassifier{topic: "gambling"}, jobrunner.NewSyncRunner(), nil)

	if _, err := svc.QualifyWebsites(context.Background(), applb.QualifyRequest{
		Sheet:          "WEBSITES",
		AcceptedTopics: []string{"gambling"},
	}); err != nil {
		t.Fatalf("QualifyWebsites: %v", err)
	}

	// Results arrive in completion (not input) order, so index by row.
	byRow := make(map[int]domain.Result, len(src.written))
	for _, r := range src.written {
		byRow[r.Row] = r
	}
	if len(byRow) != 3 {
		t.Fatalf("written rows = %d, want 3", len(byRow))
	}
	if !byRow[2].Suitable || !byRow[4].Suitable {
		t.Errorf("a.com/c.com should be suitable: %+v", byRow)
	}
	if byRow[3].Topic != "" || byRow[3].Suitable {
		t.Errorf("failed fetch (b.com) must be unsuitable with empty topic, got %+v", byRow[3])
	}
}

func TestQualifyWebsites_CanceledFetchNotWritten(t *testing.T) {
	src := &fakeSource{sites: []domain.Website{{Row: 2, URL: "https://a.com"}}}
	fetcher := scriptedFetcher{errs: map[string]error{"https://a.com": context.Canceled}}
	svc := applb.NewService(src, fetcher, fakeClassifier{topic: "gambling"}, jobrunner.NewSyncRunner(), nil)

	if _, err := svc.QualifyWebsites(context.Background(), applb.QualifyRequest{
		Sheet:          "WEBSITES",
		AcceptedTopics: []string{"gambling"},
	}); err != nil {
		t.Fatalf("QualifyWebsites: %v", err)
	}
	// A cancellation must abort, not write a misleading "unsuitable" verdict.
	if len(src.written) != 0 {
		t.Errorf("canceled fetch must not write a verdict, got %d rows", len(src.written))
	}
}

func TestQualifyWebsites_Validation(t *testing.T) {
	svc := applb.NewService(&fakeSource{}, fakeFetcher{}, fakeClassifier{}, jobrunner.NewSyncRunner(), nil)
	if _, err := svc.QualifyWebsites(context.Background(), applb.QualifyRequest{AcceptedTopics: []string{"x"}}); err != applb.ErrNoSheet {
		t.Errorf("missing sheet err = %v, want ErrNoSheet", err)
	}
	if _, err := svc.QualifyWebsites(context.Background(), applb.QualifyRequest{Sheet: "WEBSITES"}); err != applb.ErrNoTopics {
		t.Errorf("missing topics err = %v, want ErrNoTopics", err)
	}
}
