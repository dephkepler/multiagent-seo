package linkbuilding

import "context"

// WebsiteSource reads donor candidates from a named sheet/tab and writes the
// qualification results back to the same rows. Implemented by a Google Sheets
// adapter; the tab name is chosen per run, so the source stays stateless about
// which list it's processing.
type WebsiteSource interface {
	List(ctx context.Context, sheet string) ([]Website, error)
	WriteResults(ctx context.Context, sheet string, results []Result) error
}

// PageFetcher fetches a site's homepage and extracts the signals we need
// (title/meta/headings/text for classification, raw links for the domain count).
type PageFetcher interface {
	Fetch(ctx context.Context, url string) (Page, error)
}

// TopicClassifier picks the page's main topic from the candidate list, or
// returns an empty topic when none fit. Implemented by an LLM adapter.
type TopicClassifier interface {
	Classify(ctx context.Context, page Page, candidates []string) (string, error)
}
