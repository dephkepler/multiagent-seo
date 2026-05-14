package application

import (
	"context"
	"fmt"
	"time"

	"contentflow/internal/llm"
	"contentflow/internal/prompt"
	"contentflow/internal/server"
)

// resolvedJob holds all per-request settings after merging with config defaults.
type resolvedJob struct {
	keyword     string
	minWords    int
	maxWords    int
	maxTokens   int
	language    string
	siteTopic   string
	extraRules  string
	cluster     prompt.Cluster
	client      llm.Client
	provider    string
	model       string
	maxCycles   int
	aiThreshold float64
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
	job.provider = provider
	job.model = model
	job.maxCycles = pickInt(req.MaxCycles, a.cfg.Checker.MaxCycles)
	job.aiThreshold = pickFloat(req.AIThreshold, a.cfg.Checker.AIThreshold)

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

// backgroundJobTimeout is the upper bound for one generation pipeline.
// Long enough for 5000-word articles with multiple humanize cycles on
// the slowest provider; aborted with context cancellation past this.
const backgroundJobTimeout = 15 * time.Minute

// processKeyword does the synchronous prep — Sheets lookup, DB row,
// LLM client setup — then kicks the long-running pipeline off in a
// goroutine and returns 202 Accepted immediately. Clients poll
// GET /articles/{id} to learn when generation finished.
func (a *Application) processKeyword(ctx context.Context, req server.GenerateRequest) (*server.GenerateAccepted, error) {
	job, err := a.resolveJob(ctx, req)
	if err != nil {
		return nil, err
	}

	articleID, err := a.repo.CreateArticle(ctx, job.keyword)
	if err != nil {
		a.log.Error("create article in db", "keyword", job.keyword, "err", err)
		return nil, fmt.Errorf("create article: %w", err)
	}

	article, err := a.repo.GetArticle(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("fetch article: %w", err)
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		bgCtx, cancel := context.WithTimeout(context.Background(), backgroundJobTimeout)
		defer cancel()
		a.runGeneration(bgCtx, articleID, job, req.AutoPublish)
	}()

	a.log.Info("generation accepted",
		"article_id", articleID,
		"keyword", job.keyword,
		"provider", job.provider,
		"model", job.model,
		"target_keywords", len(job.cluster.Keywords),
	)

	return &server.GenerateAccepted{
		ID:             article.ID,
		Keyword:        article.Keyword,
		Status:         article.Status,
		CreatedAt:      article.CreatedAt,
		TargetKeywords: job.cluster.Keywords,
		SuggestedTitle: job.cluster.Title,
		Provider:       job.provider,
		Model:          job.model,
		StatusURL:      fmt.Sprintf("/articles/%d", article.ID),
	}, nil
}

// runGeneration is the body of the background goroutine. It runs the
// full generation pipeline, persists every result (status, WP urls,
// competitor data, check result) to the database, and handles
// auto-publish. Nothing is returned to a caller; clients learn the
// outcome via GET /articles/{id}.
func (a *Application) runGeneration(ctx context.Context, articleID int64, job resolvedJob, autoPublish bool) {
	out, err := a.generate(ctx, articleID, job)
	if err != nil {
		a.log.Error("generation failed", "keyword", job.keyword, "article_id", articleID, "err", err)
		if markErr := a.repo.MarkFailed(ctx, articleID); markErr != nil {
			a.log.Error("mark article failed", "article_id", articleID, "err", markErr)
		}
		return
	}

	if autoPublish {
		if !out.checkPassed {
			a.log.Warn("auto-publish blocked: originality check failed", "article_id", articleID)
		} else if _, pubErr := a.publishArticle(ctx, articleID); pubErr != nil {
			a.log.Error("auto-publish failed", "article_id", articleID, "err", pubErr)
		}
	}

	a.log.Info("generation persisted",
		"article_id", articleID,
		"keyword", job.keyword,
		"duration_ms", out.durationMS,
		"total_input_tokens", out.tokenSummary.TotalInput,
		"total_output_tokens", out.tokenSummary.TotalOutput,
	)
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

func pickFloat(req, def float64) float64 {
	if req != 0 {
		return req
	}
	return def
}
