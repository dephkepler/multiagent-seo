package articles

import (
	"context"
	"time"

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
	Topics(ctx context.Context) ([]string, error)
	// Clusters returns every topic's cluster in a single fetch, keyed by
	// normalized topic, so batch generation avoids one Lookup per topic.
	Clusters(ctx context.Context) (map[string]Cluster, error)
}

// TopicSourceProvider resolves the TopicSource a site's keywords should come
// from for a given language — a site with its own configured keyword sheet
// for that language gets a source scoped to that sheet, anything else falls
// back to a shared default.
type TopicSourceProvider interface {
	ForSite(ctx context.Context, siteID uuid.UUID, language string) (TopicSource, error)
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

// ForSite resolves the language a site publishes with — a WordPress site
// ignores it (it isn't bilingual in this system's model), MODX uses it to
// pick which context (and thus URL prefix) the article lands under.
type PublisherProvider interface {
	ForSite(ctx context.Context, siteID uuid.UUID, language string) (Publisher, error)
}

type PromptStore interface {
	ActiveVariants(ctx context.Context, stage string) ([]PromptVariant, error)
	InsertVariant(ctx context.Context, v PromptVariant) (int64, error)
	SetVariantStatus(ctx context.Context, id int64, status VariantStatus) error
	SaveOutcome(ctx context.Context, o PromptOutcome) error
	SelectionStats(ctx context.Context, stage string, since time.Time) ([]PromptVariantStat, error)
	WorstOutcomes(ctx context.Context, variantID int64, since time.Time, limit int) ([]PromptFailure, error)
	UpdateOutcomeReward(ctx context.Context, articleID int64, stage string, human *float64, weight float64) error
}
