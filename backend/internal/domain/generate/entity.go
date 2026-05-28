package generate

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusGenerating Status = "generating"
	StatusDraft      Status = "draft"
	StatusPublished  Status = "published"
	StatusFailed     Status = "failed"
)

func (s Status) IsTerminal() bool {
	return s == StatusPublished || s == StatusFailed
}

// Article is the generated piece of content tracked through its lifecycle.
// IDs stay int64 (legacy sequence); SiteID points at a wordpress_sites row,
// replacing the legacy free-form Site string which is now display-only.
type Article struct {
	ID      int64
	Keyword string
	SiteID  uuid.UUID
	Site    string
	Status  Status

	WPPostID  int64
	WPEditURL string
	WPPostURL string

	ImagesRequested int
	ImagesResolved  int
	ImagesSkipped   int

	CompetitorData json.RawMessage
	CheckResult    json.RawMessage

	CreatedAt time.Time
	UpdatedAt time.Time
}
