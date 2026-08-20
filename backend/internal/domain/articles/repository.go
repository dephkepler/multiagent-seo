package articles

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("article not found")

// ErrAlreadyPublished signals a lost race: the row was already published by
// another concurrent Publish() call between the caller's status check and
// this write (MarkPublished's UPDATE is conditional on status, see the
// postgres adapter).
var ErrAlreadyPublished = errors.New("article already published")

type CreateArticle struct {
	Keyword       string
	SiteID        uuid.UUID
	Site          string
	RequestParams json.RawMessage
}

type ArticleRepository interface {
	Create(ctx context.Context, in CreateArticle) (Article, error)
	Get(ctx context.Context, id int64) (*Article, error)
	List(ctx context.Context, limit, offset int) ([]Article, int, error)
	GeneratedKeywords(ctx context.Context, siteID uuid.UUID) ([]string, error)

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
