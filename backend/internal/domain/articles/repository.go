package articles

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("article not found")

type CreateArticle struct {
	Keyword       string
	SiteID        uuid.UUID
	Site          string
	RequestParams json.RawMessage
}

type ArticleRepository interface {
	Create(ctx context.Context, in CreateArticle) (int64, error)
	Get(ctx context.Context, id int64) (*Article, error)
	List(ctx context.Context, limit, offset int) ([]Article, int, error)

	UpdateDraft(ctx context.Context, id, wpPostID int64, editURL string) error
	MarkFailed(ctx context.Context, id int64) error
	MarkPublished(ctx context.Context, id int64, postURL string) error
	SetHumanRating(ctx context.Context, id int64, rating *bool) error

	SaveImageStats(ctx context.Context, id int64, requested, resolved, skipped int) error
	SaveCompetitorData(ctx context.Context, id int64, data any) error
	SaveCheckResult(ctx context.Context, id int64, result any) error

	SaveRevision(ctx context.Context, rev Revision) (int, error)
	SaveEvent(ctx context.Context, ev GenerationEvent) error
}
