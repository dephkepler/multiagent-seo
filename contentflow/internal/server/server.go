package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"contentflow/internal/repo"
)

const maxKeywordLength = 200

// GenerateRequest is the typed input for a generation call. All optional
// fields fall back to config defaults when zero-valued.
type GenerateRequest struct {
	Keyword    string
	MinWords   int
	MaxWords   int
	MaxTokens  int    // hard cap on LLM output tokens; 0 = derive or provider default
	Language   string // "" = config default
	SiteTopic  string // "" = config default
	ExtraRules string // "" = config default
	Provider   string // "" = config default (groq / claude / anthropic)
	Model      string // "" = config default
}

// GenerateResult mirrors application.GenerateResult so the server
// layer is decoupled from the application package's types.
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
}

type GenerateFunc func(ctx context.Context, req GenerateRequest) (*GenerateResult, error)

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
	log      *slog.Logger
	http     *http.Server
}

func New(repo *repo.Repo, generate GenerateFunc, log *slog.Logger, cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 10 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 5 * time.Minute
	}

	s := &Server{repo: repo, generate: generate, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /generate", s.handleGenerate)
	mux.HandleFunc("GET /articles", s.handleArticles)

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
		Keyword    string `json:"keyword"`
		MinWords   int    `json:"min_words"`
		MaxWords   int    `json:"max_words"`
		MaxTokens  int    `json:"max_tokens"`
		Language   string `json:"language"`
		SiteTopic  string `json:"site_topic"`
		ExtraRules string `json:"extra_rules"`
		Provider   string `json:"provider"`
		Model      string `json:"model"`
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
		Keyword:    body.Keyword,
		MinWords:   body.MinWords,
		MaxWords:   body.MaxWords,
		MaxTokens:  body.MaxTokens,
		Language:   body.Language,
		SiteTopic:  body.SiteTopic,
		ExtraRules: body.ExtraRules,
		Provider:   body.Provider,
		Model:      body.Model,
	})
	if err != nil {
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(articles)
}
