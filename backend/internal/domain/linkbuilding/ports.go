package linkbuilding

import "context"

type WebsiteSource interface {
	List(ctx context.Context, sheet string) ([]Website, error)
	WriteResults(ctx context.Context, sheet string, results []Result) error
}

type PageFetcher interface {
	Fetch(ctx context.Context, url string) (Page, error)
}

type TopicClassifier interface {
	Classify(ctx context.Context, page Page, candidates []string) (string, error)
}

type CredentialSource interface {
	ListCredentials(ctx context.Context, sheet string) ([]SiteCredential, error)
	WriteLoginStatus(ctx context.Context, sheet string, results []LoginResult) error
	ClearStaleStatuses(ctx context.Context, sheet string) error
}

type SiteAuthenticator interface {
	Login(ctx context.Context, cred SiteCredential) (LoginResult, error)
}

type PlacementSink interface {
	WritePlacementStatus(ctx context.Context, sheet string, results []PlacementResult) error
}

type DonorCredentialStore interface {
	Get(ctx context.Context, donorURL string) (DonorCredential, bool, error)
	Save(ctx context.Context, cred DonorCredential) error
}

type DonorAppPasswordIssuer interface {
	IssueAppPassword(ctx context.Context, donorURL, login, password string) (string, error)
}

type DonorPostEditor interface {
	LatestPost(ctx context.Context, cred DonorCredential) (DonorPost, error)
	UpdatePostContent(ctx context.Context, cred DonorCredential, postID int64, newHTML string) error
}

type BacklinkPlacer interface {
	Place(ctx context.Context, html, targetURL string) (BacklinkInsertion, error)
}
