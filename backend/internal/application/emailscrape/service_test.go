package emailscrape

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	domain "multiagent-seo/internal/domain/emailscrape"
	"multiagent-seo/pkg/jobrunner"
)

var testSettings = Settings{
	Concurrency:     2,
	FlushBatch:      10,
	WriteTimeout:    5 * time.Second,
	MaxContactPages: 3,
	PerHostRPS:      1000,
	PerHostBurst:    1000,
}

type fakeSites struct {
	list    []domain.Site
	mu      sync.Mutex
	written map[int]domain.Result
}

func (f *fakeSites) List(context.Context, string) ([]domain.Site, error) { return f.list, nil }
func (f *fakeSites) WriteResults(_ context.Context, _ string, results []domain.Result) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.written == nil {
		f.written = map[int]domain.Result{}
	}
	for _, r := range results {
		f.written[r.Row] = r
	}
	return nil
}

type fakeFetcher struct {
	pages map[string]domain.Page
}

func (f *fakeFetcher) Fetch(_ context.Context, url string) (domain.Page, error) {
	return f.pages[url], nil
}

type fakeStore struct {
	mu      sync.Mutex
	records []domain.EmailRecord
}

func (f *fakeStore) Save(_ context.Context, records []domain.EmailRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, records...)
	return nil
}

type fakeJobs struct{}

func (fakeJobs) Create(context.Context, string, int) (string, error)  { return "job-1", nil }
func (fakeJobs) UpdateProgress(context.Context, string, int) error    { return nil }
func (fakeJobs) Finish(context.Context, string, string, string) error { return nil }
func (fakeJobs) Get(context.Context, string) (domain.Job, error)      { return domain.Job{}, nil }

func TestScrapeEmails_HomeAndContactPage(t *testing.T) {
	sites := &fakeSites{list: []domain.Site{
		{Row: 1, URL: "http://a.com"},
		{Row: 2, URL: "http://b.com"},
	}}
	fetcher := &fakeFetcher{pages: map[string]domain.Page{
		"http://a.com":         {URL: "http://a.com", HTML: `info@a.com <a href="/contact">c</a>`, Links: []domain.Link{{Href: "/contact"}}},
		"http://a.com/contact": {URL: "http://a.com/contact", HTML: `sales@a.com`},
		"http://b.com":         {URL: "http://b.com", HTML: `nothing useful here`},
	}}
	store := &fakeStore{}

	svc := NewService(sites, fetcher, store, fakeJobs{}, jobrunner.NewSyncRunner(), nil, testSettings)

	res, err := svc.ScrapeEmails(context.Background(), ScrapeRequest{Sheet: "list1"})
	if err != nil {
		t.Fatalf("ScrapeEmails: %v", err)
	}
	if res.SitesQueued != 2 {
		t.Errorf("SitesQueued = %d, want 2", res.SitesQueued)
	}
	if res.JobID != "job-1" {
		t.Errorf("JobID = %q, want job-1", res.JobID)
	}

	a := sites.written[1]
	if want := []string{"info@a.com", "sales@a.com"}; !slices.Equal(a.Emails, want) {
		t.Errorf("a.com emails = %v, want %v", a.Emails, want)
	}
	if a.Status != "found 2" {
		t.Errorf("a.com status = %q, want \"found 2\"", a.Status)
	}

	b := sites.written[2]
	if len(b.Emails) != 0 || b.Status != "no emails" {
		t.Errorf("b.com = %+v, want no emails", b)
	}

	if len(store.records) != 2 {
		t.Errorf("store records = %d, want 2", len(store.records))
	}
}

func TestScrapeEmails_NoSheet(t *testing.T) {
	svc := NewService(&fakeSites{}, &fakeFetcher{}, &fakeStore{}, fakeJobs{}, jobrunner.NewSyncRunner(), nil, testSettings)
	if _, err := svc.ScrapeEmails(context.Background(), ScrapeRequest{}); err != ErrNoSheet {
		t.Errorf("err = %v, want ErrNoSheet", err)
	}
}
