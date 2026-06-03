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

// CredentialSource reads the site-login inventory from a named sheet/tab and
// writes the per-site login status back to the same rows. Like WebsiteSource
// the tab is chosen per run, so the source stays stateless about which list it
// processes.
type CredentialSource interface {
	ListCredentials(ctx context.Context, sheet string) ([]SiteCredential, error)
	WriteLoginStatus(ctx context.Context, sheet string, results []LoginResult) error
	// ClearStaleStatuses blanks the status column of rows that are no longer
	// suitable, so a leftover verdict from an earlier run doesn't linger on a
	// site we no longer log into.
	ClearStaleStatuses(ctx context.Context, sheet string) error
}

// SiteAuthenticator performs a fresh login to a site and reports whether a
// session was established. A non-nil error is reserved for context
// cancellation; ordinary outcomes (bad credentials, unreachable) are reported
// as LoginResult.OK=false so the caller records a status instead of aborting.
type SiteAuthenticator interface {
	Login(ctx context.Context, cred SiteCredential) (LoginResult, error)
}

// PlacementSink writes the Flow 3 per-row outcome ("placed: <edit_url>" or a
// failure reason) into the sheet's I column. Kept separate from
// CredentialSource because it's only used by the backlink placement run.
type PlacementSink interface {
	WritePlacementStatus(ctx context.Context, sheet string, results []PlacementResult) error
}

// DonorCredentialStore caches the app-password we issued for each donor so the
// next run skips the legacy form login. The store handles the at-rest
// encryption of AppPassword.
type DonorCredentialStore interface {
	Get(ctx context.Context, donorURL string) (DonorCredential, bool, error)
	Save(ctx context.Context, cred DonorCredential) error
}

// DonorAppPasswordIssuer logs into a donor's WordPress with the legacy
// username + password from the sheet, then asks WP to mint a fresh
// Application Password we can use for REST API auth on every future call.
type DonorAppPasswordIssuer interface {
	IssueAppPassword(ctx context.Context, donorURL, login, password string) (string, error)
}

// DonorPostEditor talks to the donor's WP REST API using an app password.
// LatestPost returns the freshest published post; UpdatePostContent writes
// modified HTML back to the same post.
type DonorPostEditor interface {
	LatestPost(ctx context.Context, cred DonorCredential) (DonorPost, error)
	UpdatePostContent(ctx context.Context, cred DonorCredential, postID int64, newHTML string) error
}

// BacklinkPlacer asks an LLM to insert a contextually-anchored link to
// targetURL into the existing post HTML, without changing the rest of the
// article. The returned ModifiedHTML must contain targetURL; the caller checks
// that as a sanity guard.
type BacklinkPlacer interface {
	Place(ctx context.Context, html, targetURL string) (BacklinkInsertion, error)
}
