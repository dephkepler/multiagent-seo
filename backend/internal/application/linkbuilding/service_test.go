package linkbuilding_test

import (
	"context"
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
func (f *fakeSource) WriteResults(_ context.Context, _ string, r []domain.Result) error {
	f.written = r
	return nil
}

type fakeFetcher struct{ page domain.Page }

func (f fakeFetcher) Fetch(context.Context, string) (domain.Page, error) { return f.page, nil }

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

func TestQualifyWebsites_Validation(t *testing.T) {
	svc := applb.NewService(&fakeSource{}, fakeFetcher{}, fakeClassifier{}, jobrunner.NewSyncRunner(), nil)
	if _, err := svc.QualifyWebsites(context.Background(), applb.QualifyRequest{AcceptedTopics: []string{"x"}}); err != applb.ErrNoSheet {
		t.Errorf("missing sheet err = %v, want ErrNoSheet", err)
	}
	if _, err := svc.QualifyWebsites(context.Background(), applb.QualifyRequest{Sheet: "WEBSITES"}); err != applb.ErrNoTopics {
		t.Errorf("missing topics err = %v, want ErrNoTopics", err)
	}
}
