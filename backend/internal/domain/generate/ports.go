package generate

import (
	"context"

	"github.com/google/uuid"
)

// LLMClient is the text-completion provider. maxTokens == 0 means provider default.
type LLMClient interface {
	Complete(ctx context.Context, prompt string, maxTokens int) (string, Usage, error)
}

// LLMFactory resolves an LLMClient for a provider/model chosen per request,
// supplying the matching API key. Keeps key/provider wiring out of the use-case.
type LLMFactory interface {
	ForModel(provider, model string) (LLMClient, error)
}

// SERPProvider fetches competitor SERP data for a keyword.
type SERPProvider interface {
	GetSERP(ctx context.Context, keyword, languageCode string, limit int) (*CompetitorData, error)
}

// TopicSource resolves the keyword cluster and H1 for a topic.
type TopicSource interface {
	Lookup(ctx context.Context, topic string) (Cluster, error)
}

// ContentChecker scores generated content for AI/plagiarism originality.
type ContentChecker interface {
	Check(ctx context.Context, content string) (*CheckResult, error)
}

// ImageResolver turns an [IMG | ...] placeholder into a real image plus
// attribution metadata. Implementations must be safe for concurrent use.
// A Resolve error is non-fatal: callers strip the placeholder and continue.
// keyword is the article's target keyword so resolvers can build a topical query.
type ImageResolver interface {
	Resolve(ctx context.Context, keyword, description, alt string) (ResolvedImage, error)
}

type Publisher interface {
	CreateDraft(ctx context.Context, p Post) (postID int64, editURL string, err error)
	Publish(ctx context.Context, postID int64) (postURL string, err error)
}

// PublisherProvider builds a Publisher bound to a specific WordPress site,
// resolving (and decrypting) that site's credentials. Keeps the application
// layer from importing the concrete WordPress/credentials infrastructure.
type PublisherProvider interface {
	ForSite(ctx context.Context, siteID uuid.UUID) (Publisher, error)
}
