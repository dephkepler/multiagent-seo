package emailscrape

import "context"

type SiteSource interface {
	// List returns sites that still need scraping: rows with a URL and no status yet.
	List(ctx context.Context, sheet string) ([]Site, error)
	WriteResults(ctx context.Context, sheet string, results []Result) error
}

type PageFetcher interface {
	Fetch(ctx context.Context, url string) (Page, error)
}

type EmailStore interface {
	Save(ctx context.Context, records []EmailRecord) error
}
