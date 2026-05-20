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

// ErrNoCluster signals a Sheets lookup returned no keywords for the topic;
// the HTTP layer maps it to 404 before any tokens are spent.
var ErrNoCluster = errors.New("no keyword cluster for topic")

// Sentinel errors so the HTTP layer can map publish failures to precise
// status codes without string matching.
var (
	ErrArticleNotFound  = errors.New("article not found")
	ErrAlreadyPublished = errors.New("article already published")
	ErrNoDraftToPublish = errors.New("article has no WordPress draft to publish")
	ErrUnknownSite      = errors.New("unknown wordpress site")
)

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Keyword string `json:"keyword,omitempty"`
}

func writeJSONError(w http.ResponseWriter, status int, body errorResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (s *Server) writeJSON(w http.ResponseWriter, r *http.Request, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.log.Warn("encode response",
			"err", err,
			"path", r.URL.Path,
			"request_id", RequestIDFromContext(r.Context()),
		)
	}
}

// GenerateRequest is the typed input for a generation call. Zero values
// fall back to config defaults. Target keywords are resolved by the
// application layer via Sheets lookup using Keyword as the topic.
type GenerateRequest struct {
	Keyword     string
	Site        string
	MinWords    int
	MaxWords    int
	MaxTokens   int
	Language    string
	SiteTopic   string
	ExtraRules  string
	Provider    string
	Model       string
	AutoPublish bool
	MaxCycles   int
	AIThreshold float64
}

// GenerateAccepted is the 202 response shape for POST /generate. Generation
// runs in the background; clients poll GET /articles/{id} for completion.
type GenerateAccepted struct {
	ID        int64     `json:"id"`
	Keyword   string    `json:"keyword"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`

	// Returned eagerly so the caller can verify the cluster without waiting.
	TargetKeywords []string `json:"target_keywords"`
	SuggestedTitle string   `json:"suggested_title"`

	Site string `json:"site"`

	Provider string `json:"provider"`
	Model    string `json:"model"`

	StatusURL string `json:"status_url"`
}

type GenerateFunc func(ctx context.Context, req GenerateRequest) (*GenerateAccepted, error)

type ArticleResult struct {
	ID             int64           `json:"id"`
	Keyword        string          `json:"keyword"`
	Site           string          `json:"site"`
	Status         string          `json:"status"`
	WPPostID       int64           `json:"wp_post_id"`
	WPEditURL      string          `json:"wp_edit_url"`
	WPPostURL      string          `json:"wp_post_url"`
	CompetitorData json.RawMessage `json:"competitor_data,omitempty"`
	CheckResult    json.RawMessage `json:"check_result,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type PublishResult struct {
	ID        int64     `json:"id"`
	Site      string    `json:"site"`
	Status    string    `json:"status"`
	WPPostID  int64     `json:"wp_post_id"`
	WPEditURL string    `json:"wp_edit_url"`
	WPPostURL string    `json:"wp_post_url"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PublishFunc func(ctx context.Context, articleID int64) (*PublishResult, error)

// Config controls HTTP server binding and timeouts; zero values use defaults.
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

	appMux := http.NewServeMux()
	appMux.HandleFunc("POST /generate", s.handleGenerate)
	appMux.HandleFunc("GET /articles", s.handleArticles)
	appMux.HandleFunc("GET /articles/{id}", s.handleGetArticle)
	appMux.HandleFunc("POST /articles/{id}/publish", s.handlePublish)
	appMux.HandleFunc("GET /openapi.yaml", s.handleOpenAPISpec)
	appMux.HandleFunc("GET /docs", s.handleSwaggerUI)
	appMux.HandleFunc("GET /docs/", s.handleSwaggerUI)
	appMux.HandleFunc("GET /{$}", s.handleLandingPage)

	s.http = &http.Server{
		Addr:         cfg.Addr,
		Handler:      requestIDMiddleware(accessLogMiddleware(recoverMiddleware(appMux, log), log)),
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

// Real payloads are a few hundred bytes; the cap defends json.NewDecoder
// against accidental or malicious bloat.
const maxGenerateBodyBytes = 64 << 10

func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxGenerateBodyBytes)
	var body struct {
		Keyword     string `json:"keyword"`
		Site        string `json:"site"`
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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if body.Keyword == "" {
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
		Site:        body.Site,
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
		if errors.Is(err, ErrUnknownSite) {
			s.log.Warn("generate aborted: unknown site", "site", body.Site)
			writeJSONError(w, http.StatusBadRequest, errorResponse{
				Error:   "unknown_site",
				Message: err.Error(),
			})
			return
		}
		s.log.Error("generate", "keyword", body.Keyword, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	s.writeJSON(w, r, result)
}

func (s *Server) handleGetArticle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "id must be a positive integer", http.StatusBadRequest)
		return
	}

	article, err := s.repo.GetArticle(r.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, errorResponse{
				Error:   "not_found",
				Message: "article with this id does not exist",
			})
			return
		}
		s.log.Error("get article", "id", id, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.writeJSON(w, r, ArticleResult{
		ID:             article.ID,
		Keyword:        article.Keyword,
		Site:           article.Site,
		Status:         article.Status,
		WPPostID:       article.WPPostID,
		WPEditURL:      article.WPEditURL,
		WPPostURL:      article.WPPostURL,
		CompetitorData: article.CompetitorData,
		CheckResult:    article.CheckResult,
		CreatedAt:      article.CreatedAt,
		UpdatedAt:      article.UpdatedAt,
	})
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

	s.writeJSON(w, r, articles)
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

	s.writeJSON(w, r, result)
}
