package generate

// Usage reports token consumption from a single LLM completion.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type SERPItem struct {
	Rank        int    `json:"rank"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type FeaturedSnippet struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// CompetitorData is the SERP snapshot a SERPProvider returns for a keyword.
type CompetitorData struct {
	Keyword         string           `json:"keyword"`
	SerpDate        string           `json:"serp_date"`
	Results         []SERPItem       `json:"results"`
	PAA             []string         `json:"paa,omitempty"`
	FeaturedSnippet *FeaturedSnippet `json:"featured_snippet,omitempty"`
}

// Cluster holds the target keywords and H1 a TopicSource resolves for a topic.
// Empty Keywords with no error means the topic has no row.
type Cluster struct {
	Keywords []string
	Title    string // article H1; when set the LLM must use exact wording
}

type CheckResult struct {
	AIScore          float64  `json:"ai_score"`
	PlagiarismScore  float64  `json:"plagiarism_score"`
	Original         bool     `json:"original"`
	Provider         string   `json:"provider"`
	Issues           []string `json:"issues,omitempty"`
	SentencesFlagged []string `json:"sentences_flagged,omitempty"`
	ReportURL        string   `json:"report_url,omitempty"`
}

// Post is the rendered article handed to a Publisher. Content is opaque
// payload already in the destination's native body format (HTML for
// WordPress). SEOTitle/SEODesc are plain-text meta fields the implementation
// maps to its SEO plugin; they never appear in Content. Status is the
// CMS-native status string; empty lets the implementation choose.
type Post struct {
	Title    string
	Content  string
	SEOTitle string
	SEODesc  string
	Status   string
}

// ResolvedImage is what an ImageResolver returns. URL is required; the rest
// drive the attribution figcaption.
type ResolvedImage struct {
	URL             string
	Photographer    string
	PhotographerURL string
	SourceURL       string
}

// RenderStats reports per-render image accounting so callers can persist it.
type RenderStats struct {
	ImagesRequested int // [IMG | ...] placeholders the LLM emitted
	ImagesResolved  int // placeholders that got a real image URL
	ImagesSkipped   int // requested - resolved (no resolver, error, empty URL)
}
