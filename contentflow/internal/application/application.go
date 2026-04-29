package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"contentflow/internal/config"
	"contentflow/internal/llm"
	"contentflow/internal/prompt"
	"contentflow/internal/repo"
	"contentflow/internal/server"
	"contentflow/internal/sheets"
	"contentflow/internal/wordpress"
)

type Application struct {
	cfg  *config.Config
	log  *slog.Logger
	repo *repo.Repo
	llm  llm.Client
	wp   wordpress.Client
	sh   sheets.Client
	srv  *server.Server
	wg   sync.WaitGroup
}

func New(log *slog.Logger) *Application {
	return &Application{log: log}
}

func (a *Application) Start(ctx context.Context) error {
	if err := a.initConfig(); err != nil {
		return fmt.Errorf("init config: %w", err)
	}
	if err := a.initRepo(ctx); err != nil {
		return fmt.Errorf("init repo: %w", err)
	}
	if err := a.initSheets(ctx); err != nil {
		return fmt.Errorf("init sheets: %w", err)
	}
	if err := a.initLLM(); err != nil {
		return fmt.Errorf("init llm: %w", err)
	}
	if err := a.initWordPress(); err != nil {
		return fmt.Errorf("init wordpress: %w", err)
	}
	if err := a.initServer(); err != nil {
		return fmt.Errorf("init server: %w", err)
	}

	a.log.Info("contentflow started")
	return nil
}

func (a *Application) Wait(ctx context.Context, cancel context.CancelFunc) error {
	defer cancel()
	<-ctx.Done()
	a.log.Info("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := a.srv.Shutdown(shutdownCtx); err != nil {
		a.log.Error("http shutdown", "err", err)
	}

	a.repo.Close()
	a.wg.Wait()
	a.log.Info("graceful shutdown completed")
	return nil
}

// resolvedJob holds all per-request settings after merging with config defaults.
type resolvedJob struct {
	keyword    string
	minWords   int
	maxWords   int
	maxTokens  int
	language   string
	siteTopic  string
	extraRules string
	cluster    prompt.Cluster
	client     llm.Client
}

func (a *Application) resolveJob(ctx context.Context, req server.GenerateRequest) (resolvedJob, error) {
	art := a.cfg.Article

	job := resolvedJob{
		keyword:    req.Keyword,
		minWords:   pickInt(req.MinWords, art.MinWords),
		maxWords:   pickInt(req.MaxWords, art.MaxWords),
		language:   pickStr(req.Language, art.Language),
		siteTopic:  pickStr(req.SiteTopic, art.SiteTopic),
		extraRules: pickStr(req.ExtraRules, art.ExtraRules),
	}

	// Look up target-keyword cluster from Sheets using the article topic.
	// Require a non-empty cluster: generating without targets wastes tokens
	// on what is almost certainly a typo or a topic missing from the sheet.
	res, err := a.sh.Lookup(ctx, req.Keyword)
	if err != nil {
		return resolvedJob{}, fmt.Errorf("sheets lookup: %w", err)
	}
	if len(res.Keywords) == 0 {
		a.log.Warn("no keyword cluster for topic", "keyword", req.Keyword)
		return resolvedJob{}, server.ErrNoCluster
	}
	a.log.Info("keyword cluster loaded",
		"keyword", req.Keyword,
		"count", len(res.Keywords),
		"has_title", res.Title != "",
	)
	job.cluster = prompt.Cluster{Keywords: res.Keywords, Title: res.Title}

	// Derive a max-tokens cap: explicit wins, otherwise a buffered guess
	// from max_words (≈3 tokens per word + 200 overhead). 0 means "no cap" for Groq
	// and triggers the Claude 4096 default.
	if req.MaxTokens > 0 {
		job.maxTokens = req.MaxTokens
	} else if job.maxWords > 0 {
		job.maxTokens = job.maxWords*3 + 200
	}

	provider := pickStr(req.Provider, a.cfg.LLM.Provider)
	model := pickStr(req.Model, a.cfg.LLM.Model)

	if provider == a.cfg.LLM.Provider && model == a.cfg.LLM.Model {
		job.client = a.llm
		return job, nil
	}

	apiKey := a.cfg.LLM.KeyFor(provider)
	if apiKey == "" {
		return resolvedJob{}, fmt.Errorf("no API key configured for provider %q", provider)
	}
	client, err := llm.New(provider, apiKey, model, a.log)
	if err != nil {
		return resolvedJob{}, fmt.Errorf("build llm client: %w", err)
	}
	job.client = client
	return job, nil
}

func (a *Application) processKeyword(ctx context.Context, req server.GenerateRequest) (*server.GenerateResult, error) {
	job, err := a.resolveJob(ctx, req)
	if err != nil {
		return nil, err
	}

	articleID, err := a.repo.CreateArticle(ctx, job.keyword)
	if err != nil {
		a.log.Error("create article in db", "keyword", job.keyword, "err", err)
		return nil, fmt.Errorf("create article: %w", err)
	}

	content, err := a.generate(ctx, articleID, job)
	if err != nil {
		a.log.Error("generation failed", "keyword", job.keyword, "err", err)
		if markErr := a.repo.MarkFailed(ctx, articleID); markErr != nil {
			a.log.Error("mark article failed", "article_id", articleID, "err", markErr)
		}
		return nil, err
	}

	// Optional auto-publish: flip the draft to published before returning.
	// We tolerate publish failures — the draft already exists and the
	// caller can retry via POST /articles/{id}/publish.
	var autoPublishErr string
	if req.AutoPublish {
		if _, pubErr := a.publishArticle(ctx, articleID); pubErr != nil {
			a.log.Error("auto-publish failed", "article_id", articleID, "err", pubErr)
			autoPublishErr = pubErr.Error()
		}
	}

	article, err := a.repo.GetArticle(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("fetch article: %w", err)
	}

	return &server.GenerateResult{
		ID:               article.ID,
		Keyword:          article.Keyword,
		Status:           article.Status,
		WPPostID:         article.WPPostID,
		WPEditURL:        article.WPEditURL,
		WPPostURL:        article.WPPostURL,
		Content:          content,
		CreatedAt:        article.CreatedAt,
		UpdatedAt:        article.UpdatedAt,
		TargetKeywords:   job.cluster.Keywords,
		SuggestedTitle:   job.cluster.Title,
		AutoPublishError: autoPublishErr,
	}, nil
}

func (a *Application) generate(ctx context.Context, articleID int64, job resolvedJob) (string, error) {
	a.log.Info("step 1/3: brief", "keyword", job.keyword)
	brief, err := job.client.Complete(ctx, prompt.Brief(job.keyword, job.language, job.siteTopic, job.extraRules), job.maxTokens)
	if err != nil {
		return "", fmt.Errorf("brief: %w", err)
	}

	a.log.Info("step 2/3: writing", "keyword", job.keyword)
	article, err := job.client.Complete(ctx, prompt.Writer(brief, job.keyword, job.language, job.minWords, job.maxWords, job.cluster), job.maxTokens)
	if err != nil {
		return "", fmt.Errorf("writer: %w", err)
	}

	a.log.Info("step 3/3: editing", "keyword", job.keyword)
	edited, err := job.client.Complete(ctx, prompt.Editor(article, job.keyword, job.minWords, job.maxWords, job.cluster), job.maxTokens)
	if err != nil {
		return "", fmt.Errorf("editor: %w", err)
	}

	postID, editURL, err := a.wp.CreateDraft(ctx, wordpress.Post{
		Title:   job.keyword,
		Content: edited,
	})
	if err != nil {
		return "", fmt.Errorf("wordpress draft: %w", err)
	}

	if err := a.repo.UpdateDraft(ctx, articleID, postID, editURL); err != nil {
		return "", fmt.Errorf("update db: %w", err)
	}

	a.log.Info("done", "keyword", job.keyword, "post_id", postID, "edit_url", editURL)
	return edited, nil
}

// publishArticle flips a draft article to published state by calling the
// WordPress REST API and persisting the returned public URL.
func (a *Application) publishArticle(ctx context.Context, id int64) (*server.PublishResult, error) {
	article, err := a.repo.GetArticle(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, server.ErrArticleNotFound
		}
		return nil, fmt.Errorf("fetch article: %w", err)
	}

	switch article.Status {
	case repo.StatusPublished:
		return nil, server.ErrAlreadyPublished
	case repo.StatusGenerating, repo.StatusFailed:
		return nil, server.ErrNoDraftToPublish
	}
	if article.WPPostID == 0 {
		return nil, server.ErrNoDraftToPublish
	}

	postURL, err := a.wp.Publish(ctx, article.WPPostID)
	if err != nil {
		return nil, fmt.Errorf("wordpress publish: %w", err)
	}

	if err := a.repo.MarkPublished(ctx, id, postURL); err != nil {
		return nil, fmt.Errorf("mark published: %w", err)
	}

	updated, err := a.repo.GetArticle(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("fetch article after publish: %w", err)
	}

	a.log.Info("article published",
		"article_id", id,
		"wp_post_id", updated.WPPostID,
		"wp_post_url", updated.WPPostURL,
	)

	return &server.PublishResult{
		ID:        updated.ID,
		Status:    updated.Status,
		WPPostID:  updated.WPPostID,
		WPEditURL: updated.WPEditURL,
		WPPostURL: updated.WPPostURL,
		UpdatedAt: updated.UpdatedAt,
	}, nil
}

func pickStr(req, def string) string {
	if req != "" {
		return req
	}
	return def
}

func pickInt(req, def int) int {
	if req != 0 {
		return req
	}
	return def
}

func (a *Application) initConfig() error {
	cfg, err := config.NewConfig()
	if err != nil {
		return err
	}
	a.cfg = cfg
	return nil
}

func (a *Application) initRepo(ctx context.Context) error {
	r, err := repo.New(ctx, a.cfg.Database.URL)
	if err != nil {
		return err
	}
	a.repo = r
	return nil
}

func (a *Application) initSheets(ctx context.Context) error {
	if a.cfg.Sheets.CredentialsFile == "" || a.cfg.Sheets.SpreadsheetID == "" {
		a.log.Warn("sheets not configured — using in-memory mock (no cluster lookup)")
		a.sh = sheets.NewMock()
		return nil
	}
	c, err := sheets.New(ctx, a.cfg.Sheets, a.log)
	if err != nil {
		return err
	}
	a.sh = c
	a.log.Info("sheets configured", "spreadsheet_id", a.cfg.Sheets.SpreadsheetID, "sheet", a.cfg.Sheets.Sheet)
	return nil
}

func (a *Application) initLLM() error {
	client, err := llm.New(a.cfg.LLM.Provider, a.cfg.LLM.APIKey, a.cfg.LLM.Model, a.log)
	if err != nil {
		return err
	}
	a.llm = client
	return nil
}

func (a *Application) initWordPress() error {
	a.wp = wordpress.New(a.cfg.WordPress)
	return nil
}

func (a *Application) initServer() error {
	a.srv = server.New(a.repo, a.processKeyword, a.publishArticle, a.log, server.Config{
		Addr:         a.cfg.Server.Addr,
		ReadTimeout:  a.cfg.Server.ReadTimeout,
		WriteTimeout: a.cfg.Server.WriteTimeout,
	})

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.log.Error("http server", "err", err)
		}
	}()

	return nil
}
