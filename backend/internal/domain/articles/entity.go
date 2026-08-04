package articles

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

type Article struct {
	ID              int64
	Keyword         string
	SiteID          uuid.UUID
	Site            string
	Status          Status
	WPPostID        int64
	WPEditURL       string
	WPPostURL       string
	ImagesRequested int
	ImagesResolved  int
	ImagesSkipped   int
	CompetitorData  json.RawMessage
	CheckResult     json.RawMessage
	RequestParams   json.RawMessage
	CreatedAt       time.Time
	UpdatedAt       time.Time
	PublishedAt     *time.Time
	HumanRating     *bool // copywriter 👍/👎; nil = not rated

	// Writer-stage outcome metrics, populated only by Get (detail view).
	AIScore        *float64
	Reward         *float64
	QualityOK      *bool
	HumanizeCycles *int
	Tokens         *int
}

type RevisionSource string

const (
	RevisionGenerated RevisionSource = "generated"
	RevisionImproved  RevisionSource = "claude_improved"
	RevisionManual    RevisionSource = "manual"
)

type Revision struct {
	ID             int64
	ArticleID      int64
	Version        int
	Source         RevisionSource
	ContentMD      string
	ContentHTML    string
	SEOTitle       string
	SEODescription string
	WordCount      int
	CreatedAt      time.Time
}

const (
	StageSERP     = "serp"
	StageBrief    = "brief"
	StageWrite    = "write"
	StageEdit     = "edit"
	StageCheck    = "check"
	StageHumanize = "humanize"
	StageImages   = "images"
	StageDraft    = "draft"
	StagePublish  = "publish"
)

type GenerationEvent struct {
	ArticleID    int64
	Stage        string
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
	LatencyMS    int64
	OK           bool
}

const (
	PromptStageBrief    = "brief"
	PromptStageWriter   = "writer"
	PromptStageEditor   = "editor"
	PromptStageHumanize = "humanize"
)

type VariantStatus string

const (
	VariantChampion  VariantStatus = "champion"
	VariantCandidate VariantStatus = "candidate"
	VariantRetired   VariantStatus = "retired"
)

const (
	OriginSeed      = "seed"
	OriginGenerated = "generated"
)

type PromptVariant struct {
	ID        int64
	Stage     string        // e.g., "brief", "writer", "editor", "humanize"
	Body      string        // the actual prompt text
	Status    VariantStatus // champion, candidate, or retired
	Origin    string        // "seed" for initial variants, "generated" for those created by mutating champions
	ParentID  *int64        // if generated, the ID of the champion it was derived from
	CreatedAt time.Time
}

type PromptOutcome struct {
	ArticleID      int64    // the article for which this prompt variant was used
	Stage          string   // the stage of generation (e.g., "brief", "writer", "editor", "humanize")
	VariantID      *int64   // the specific prompt variant used, if applicable
	Reward         *float64 // bandit reward (0..1); currently 1 - ai_score, later may come from human feedback
	AIScore        *float64 // raw AI-detector score, if available
	QualityOK      bool     // whether the output passed a quality check (e.g., content checker)
	HumanizeCycles int      // number of times the content was sent for humanization (if applicable)
	Tokens         int      // total tokens used in this stage (input + output)
}

type PromptVariantStat struct {
	Variant       PromptVariant // the variant being evaluated
	Samples       int           // number of times this variant was selected for generation
	SumReward     float64       // sum of rewards for this variant
	SumComplement float64       // sum of (1 - reward) for this variant
}

type PromptFailure struct {
	ArticleID int64    // the article whose text scored as AI-written
	AIScore   float64  // the article's AI-detector score (higher = more machine-like)
	Flagged   []string // the AI-flagged sentences from the article (detector's sentences_flagged)
}
