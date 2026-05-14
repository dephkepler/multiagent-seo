package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"contentflow/internal/repo"
)

const maxKeywordLength = 200

// ErrNoCluster is returned by GenerateFunc when the Sheets lookup returns
// no keywords for the requested topic. The HTTP layer maps it to 404 so
// the client learns about the gap before any tokens are spent.
var ErrNoCluster = errors.New("no keyword cluster for topic")

// Errors the publish flow can raise. Kept as sentinels so the HTTP layer
// can map them to precise status codes without string matching.
var (
	ErrArticleNotFound  = errors.New("article not found")
	ErrAlreadyPublished = errors.New("article already published")
	ErrNoDraftToPublish = errors.New("article has no WordPress draft to publish")
)

// errorResponse is the JSON shape returned for non-2xx responses that carry
// a machine-readable reason. Plain-text responses (400 validation, 500
// internal) stay as text/plain for simplicity.
type errorResponse struct {
	Error   string `json:"error"`             // stable code, e.g. "no_cluster"
	Message string `json:"message"`           // human-readable detail
	Keyword string `json:"keyword,omitempty"` // echoed back when relevant
}

func writeJSONError(w http.ResponseWriter, status int, body errorResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// GenerateRequest is the typed input for a generation call. All optional
// fields fall back to config defaults (or empty) when zero-valued.
//
// Target keywords are not part of the request: they're resolved by the
// application layer via Sheets lookup using Keyword as the topic.
type GenerateRequest struct {
	Keyword     string
	MinWords    int
	MaxWords    int
	MaxTokens   int    // hard cap on LLM output tokens; 0 = derive or provider default
	Language    string // "" = config default
	SiteTopic   string // "" = config default
	ExtraRules  string // "" = config default
	Provider    string // "" = config default (groq / claude / anthropic)
	Model       string // "" = config default
	AutoPublish  bool    // when true, publish the WP draft right after generation
	MaxCycles    int     // max humanize rewrite cycles; 0 = config default
	AIThreshold  float64 // AI detection threshold 0.0–1.0; 0 = config default
}

// GenerateResult is the synchronous response shape for POST /generate.
type GenerateResult struct {
	ID        int64     `json:"id"`
	Keyword   string    `json:"keyword"`
	Status    string    `json:"status"`
	WPPostID  int64     `json:"wp_post_id"`
	WPEditURL string    `json:"wp_edit_url"`
	WPPostURL string    `json:"wp_post_url"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// LLM provider and model used for generation.
	Provider string `json:"provider"`
	Model    string `json:"model"`

	// Sheets lookup result.
	TargetKeywords []string `json:"target_keywords"`
	SuggestedTitle string   `json:"suggested_title"`

	// SERP competitor data fetched before generation.
	CompetitorData any `json:"competitor_data,omitempty"`

	// Humanized is true when the article was rewritten at least once after failing the originality check.
	Humanized bool `json:"humanized"`

	// CheckCycles contains every check+rewrite iteration in order.
	// cycle 1 = first check, cycle 2 = after first humanize, etc.
	CheckCycles any `json:"check_cycles,omitempty"`

	// CheckResult is the final originality check result.
	CheckResult any `json:"check_result,omitempty"`

	// AutoPublishError is set when auto_publish=true but publishing failed.
	AutoPublishError string `json:"auto_publish_error,omitempty"`

	// DurationMS is the total wall-clock time of the generation pipeline in milliseconds.
	DurationMS int64 `json:"duration_ms"`

	// TokenUsage breaks down LLM token consumption per step and in total.
	TokenUsage any `json:"token_usage,omitempty"`
}

type GenerateFunc func(ctx context.Context, req GenerateRequest) (*GenerateResult, error)

// PublishResult is the shape returned by POST /articles/{id}/publish.
type PublishResult struct {
	ID        int64     `json:"id"`
	Status    string    `json:"status"`
	WPPostID  int64     `json:"wp_post_id"`
	WPEditURL string    `json:"wp_edit_url"`
	WPPostURL string    `json:"wp_post_url"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PublishFunc func(ctx context.Context, articleID int64) (*PublishResult, error)

// Config controls HTTP server binding and timeouts. Zero values fall
// back to sensible defaults in New.
type Config struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type Server struct {
	repo     *repo.Repo
	generate GenerateFunc
	publish  PublishFunc
	log      *slog.Logger
	http     *http.Server
}

func New(repo *repo.Repo, generate GenerateFunc, publish PublishFunc, log *slog.Logger, cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 10 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 5 * time.Minute
	}

	s := &Server{repo: repo, generate: generate, publish: publish, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /generate", s.handleGenerate)
	mux.HandleFunc("GET /articles", s.handleArticles)
	mux.HandleFunc("POST /articles/{id}/publish", s.handlePublish)
	mux.HandleFunc("GET /openapi.yaml", s.handleOpenAPISpec)
	mux.HandleFunc("GET /docs", s.handleSwaggerUI)
	mux.HandleFunc("GET /docs/", s.handleSwaggerUI)
	mux.HandleFunc("GET /{$}", s.handleLandingPage)

	s.http = &http.Server{
		Addr:         cfg.Addr,
		Handler:      recoverMiddleware(mux, log),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	return s
}

func (s *Server) Start() error {
	s.log.Info("http server started", "addr", s.http.Addr)
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Keyword     string `json:"keyword"`
		MinWords    int    `json:"min_words"`
		MaxWords    int    `json:"max_words"`
		MaxTokens   int    `json:"max_tokens"`
		Language    string `json:"language"`
		SiteTopic   string `json:"site_topic"`
		ExtraRules  string `json:"extra_rules"`
		Provider    string `json:"provider"`
		Model       string `json:"model"`
		AutoPublish bool   `json:"auto_publish"`
		MaxCycles   int     `json:"max_cycles"`
		AIThreshold float64 `json:"ai_threshold"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Keyword == "" {
		http.Error(w, "keyword is required", http.StatusBadRequest)
		return
	}
	if len(body.Keyword) > maxKeywordLength {
		http.Error(w, "keyword is too long", http.StatusBadRequest)
		return
	}
	if body.MinWords < 0 || body.MaxWords < 0 || body.MaxTokens < 0 {
		http.Error(w, "min_words, max_words and max_tokens must be non-negative", http.StatusBadRequest)
		return
	}
	if body.MinWords > 0 && body.MaxWords > 0 && body.MinWords > body.MaxWords {
		http.Error(w, "min_words must be <= max_words", http.StatusBadRequest)
		return
	}

	result, err := s.generate(r.Context(), GenerateRequest{
		Keyword:     body.Keyword,
		MinWords:    body.MinWords,
		MaxWords:    body.MaxWords,
		MaxTokens:   body.MaxTokens,
		Language:    body.Language,
		SiteTopic:   body.SiteTopic,
		ExtraRules:  body.ExtraRules,
		Provider:    body.Provider,
		Model:       body.Model,
		AutoPublish: body.AutoPublish,
		MaxCycles:   body.MaxCycles,
		AIThreshold: body.AIThreshold,
	})
	if err != nil {
		if errors.Is(err, ErrNoCluster) {
			s.log.Warn("generate aborted: no cluster", "keyword", body.Keyword)
			writeJSONError(w, http.StatusNotFound, errorResponse{
				Error:   "no_cluster",
				Message: "no keyword cluster found for this topic in Sheets — add rows first",
				Keyword: body.Keyword,
			})
			return
		}
		s.log.Error("generate", "keyword", body.Keyword, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleArticles(w http.ResponseWriter, r *http.Request) {
	articles, err := s.repo.ListArticles(r.Context())
	if err != nil {
		s.log.Error("list articles", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if articles == nil {
		articles = []repo.Article{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(articles)
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "id must be a positive integer", http.StatusBadRequest)
		return
	}

	result, err := s.publish(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, ErrArticleNotFound):
			writeJSONError(w, http.StatusNotFound, errorResponse{
				Error:   "not_found",
				Message: "article with this id does not exist",
			})
		case errors.Is(err, ErrAlreadyPublished):
			writeJSONError(w, http.StatusConflict, errorResponse{
				Error:   "already_published",
				Message: "article is already published",
			})
		case errors.Is(err, ErrNoDraftToPublish):
			writeJSONError(w, http.StatusBadRequest, errorResponse{
				Error:   "no_draft",
				Message: "article has no WordPress draft yet (generation likely failed)",
			})
		default:
			s.log.Error("publish", "id", id, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
