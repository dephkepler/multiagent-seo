package models

import "time"

type KeywordStatus string

const (
	KeywordStatusPending    KeywordStatus = "pending"
	KeywordStatusProcessing KeywordStatus = "processing"
	KeywordStatusDone       KeywordStatus = "done"
	KeywordStatusFailed     KeywordStatus = "failed"
)

type ArticleStatus string

const (
	ArticleStatusPending    ArticleStatus = "pending"
	ArticleStatusGenerating ArticleStatus = "generating"
	ArticleStatusDraft      ArticleStatus = "draft"
	ArticleStatusPublished  ArticleStatus = "published"
	ArticleStatusEdited     ArticleStatus = "edited"
	ArticleStatusFailed     ArticleStatus = "failed"
)

type Keyword struct {
	ID        int64
	Keyword   string
	Status    KeywordStatus
	CreatedAt time.Time
}

type Article struct {
	ID        int64
	KeywordID int64
	Keyword   string
	Brief     string
	Content   string
	Status    ArticleStatus
	WPPostID  int64
	WPPostURL string
	CreatedAt time.Time
	UpdatedAt time.Time
}
