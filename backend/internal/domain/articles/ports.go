package articles

import (
	"context"

	"github.com/google/uuid"
)

type LLMClient interface {
	Complete(ctx context.Context, prompt string, maxTokens int) (string, Usage, error)
}

type LLMFactory interface {
	ForModel(provider, model string) (LLMClient, error)
}

type SERPProvider interface {
	GetSERP(ctx context.Context, keyword, languageCode string, limit int) (*CompetitorData, error)
}

type TopicSource interface {
	Lookup(ctx context.Context, topic string) (Cluster, error)
}

type ContentChecker interface {
	Check(ctx context.Context, content string) (*CheckResult, error)
}

type ImageResolver interface {
	Resolve(ctx context.Context, keyword, description, alt string) (ResolvedImage, error)
}

type Publisher interface {
	CreateDraft(ctx context.Context, p Post) (postID int64, editURL string, err error)
	Publish(ctx context.Context, postID int64) (postURL string, err error)
}

type PublisherProvider interface {
	ForSite(ctx context.Context, siteID uuid.UUID) (Publisher, error)
}
