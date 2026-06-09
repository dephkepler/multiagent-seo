package articles

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

type CompetitorData struct {
	Keyword         string           `json:"keyword"`
	SerpDate        string           `json:"serp_date"`
	Results         []SERPItem       `json:"results"`
	PAA             []string         `json:"paa,omitempty"`
	FeaturedSnippet *FeaturedSnippet `json:"featured_snippet,omitempty"`
}

type Cluster struct {
	Keywords []string
	Title    string
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

type Post struct {
	Title    string
	Content  string
	SEOTitle string
	SEODesc  string
	Status   string
}

type ResolvedImage struct {
	URL             string
	Photographer    string
	PhotographerURL string
	SourceURL       string
}

type RenderStats struct {
	ImagesRequested int
	ImagesResolved  int
	ImagesSkipped   int
	ImagesFailed    int
}
