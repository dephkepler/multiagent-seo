package linkbuilding

import (
	"context"
	"time"
)

type CredentialSource interface {
	ListCredentials(ctx context.Context, sheet string) ([]SiteCredential, error)
}

type PlacementSink interface {
	WritePlacementStatus(ctx context.Context, sheet string, results []PlacementResult) error
}

type PlacementStore interface {
	Save(ctx context.Context, p Placement) error
	ListByRun(ctx context.Context, runID string) ([]Placement, error)
	ListPlaced(ctx context.Context, limit, offset int) ([]Placement, int, error)
	PlacedDonors(ctx context.Context, targetURL string) (map[string]bool, error)
}

type DonorCredentialStore interface {
	Get(ctx context.Context, donorURL string) (DonorCredential, bool, error)
	Save(ctx context.Context, cred DonorCredential) error
}

type DonorProfileStore interface {
	Save(ctx context.Context, p DonorProfile) error
	RecentlyFailed(ctx context.Context, lockedCutoff, failCutoff time.Time) (map[string]bool, error)
}

type DonorAppPasswordIssuer interface {
	IssueAppPassword(ctx context.Context, donorURL, login, password string) (string, error)
}

type DonorPostEditor interface {
	Capabilities(ctx context.Context, cred DonorCredential) (DonorCapabilities, error)
	FrontPage(ctx context.Context, cred DonorCredential) (DonorPost, bool, error)
	UpdatePageContent(ctx context.Context, cred DonorCredential, pageID int64, newHTML string) error
	LatestEditablePost(ctx context.Context, cred DonorCredential, caps DonorCapabilities) (DonorPost, bool, error)
	UpdatePostContent(ctx context.Context, cred DonorCredential, postID int64, newHTML string) error
	CreatePost(ctx context.Context, cred DonorCredential, title, html string) (DonorPost, error)
	LatestTitles(ctx context.Context, cred DonorCredential, n int) ([]string, error)
	VerifyLink(ctx context.Context, pageURL, targetURL string) (string, error)
}

const (
	LinkDofollow = "dofollow"
	LinkNofollow = "nofollow"
	LinkMissing  = "missing"
)

type BacklinkPlacer interface {
	Place(ctx context.Context, html, targetURL string) (BacklinkInsertion, error)
	Compose(ctx context.Context, targetURL string, titles []string) (ComposedPost, error)
}
