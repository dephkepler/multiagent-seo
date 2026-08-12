package wordpress

import (
	"time"

	"github.com/google/uuid"
)

type Provider string

const (
	ProviderWordPress Provider = "wordpress"
	ProviderMODX      Provider = "modx"
)

// LanguageConfig holds the per-language knobs a bilingual site needs:
// which MODX context a language publishes into, and which keyword sheet
// (if any) supplies its generation topics.
type LanguageConfig struct {
	ContextKey            string `json:"contextKey,omitempty"`
	KeywordsSpreadsheetID string `json:"keywordsSpreadsheetId,omitempty"`
	KeywordsSheet         string `json:"keywordsSheet,omitempty"`
}

type Site struct {
	ID        uuid.UUID
	Alias     string
	Provider  Provider
	URL       string
	Username  string
	Languages map[string]LanguageConfig

	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
