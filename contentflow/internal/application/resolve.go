package application

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"contentflow/internal/config"
	"contentflow/internal/llm"
	"contentflow/internal/prompt"
	"contentflow/internal/publisher"
	"contentflow/internal/server"
)

// resolvedJob holds per-request settings merged with config defaults.
type resolvedJob struct {
	keyword       string
	site          string
	publisher     publisher.Publisher
	minWords      int
	maxWords      int
	maxTokens     int
	language      string
	siteTopic     string
	extraRules    string
	cluster       prompt.Cluster
	client        llm.Client
	provider      string
	model         string
	maxCycles     int
	aiThreshold   float64
	includeImages bool
}

func (a *Application) resolveJob(ctx context.Context, req server.GenerateRequest) (resolvedJob, error) {
	art := a.cfg.Article

	site := pickStr(req.Site, config.DefaultSite)
	pub, ok := a.publisherFor(site)
	if !ok {
		return resolvedJob{}, fmt.Errorf("%w %q: available %v", server.ErrUnknownSite, site, a.cfg.SiteAliases())
	}

	// Default: render images when Pexels is wired up. Per-request false
	// short-circuits the resolver so [IMG] placeholders are stripped.
	includeImages := a.pexels != nil
	if req.IncludeImages != nil {
		includeImages = *req.IncludeImages
	}

	job := resolvedJob{
		keyword:       req.Keyword,
		site:          site,
		publisher:     pub,
		minWords:      pickInt(req.MinWords, art.MinWords),
		maxWords:      pickInt(req.MaxWords, art.MaxWords),
		language:      pickStr(req.Language, art.Language),
		siteTopic:     pickStr(req.SiteTopic, art.SiteTopic),
		extraRules:    pickStr(req.ExtraRules, art.ExtraRules),
		includeImages: includeImages,
	}

	// Require a non-empty cluster: missing targets almost always means a typo
	// or a topic missing from the sheet, so don't waste LLM tokens on it.
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

	// ~3 tokens/word + 200 overhead. 0 means "no cap" on Groq and falls back
	// to Claude's 4096 default.
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

// processKeyword runs the synchronous prep and returns 202 Accepted; the
// generation pipeline continues in a goroutine and is observed via GET /articles/{id}.
func (a *Application) processKeyword(ctx context.Context, req server.GenerateRequest) (*server.GenerateAccepted, error) {
	job, err := a.resolveJob(ctx, req)
	if err != nil {
		return nil, err
	}

	articleID, err := a.repo.CreateArticle(ctx, job.keyword, job.site)
	if err != nil {
		a.log.Error("create article in db", "keyword", job.keyword, "err", err)
		return nil, fmt.Errorf("create article: %w", err)
	}

	article, err := a.repo.GetArticle(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("fetch article: %w", err)
	}

	// Propagate request_id so background-goroutine logs correlate to the caller.
	requestID := server.RequestIDFromContext(ctx)
	jobLog := a.log.With(
		"request_id", requestID,
		"article_id", articleID,
		"keyword", job.keyword,
		"site", job.site,
		"provider", job.provider,
		"model", job.model,
	)

	a.wg.Go(func() {
		bgCtx := server.WithRequestID(a.bgCtx, requestID)
		bgCtx, cancel := context.WithTimeout(bgCtx, a.cfg.Server.BackgroundJobTimeout)
		defer cancel()
		runWithRecover(jobLog, articleID, func() {
			a.runGeneration(bgCtx, jobLog, articleID, job, req.AutoPublish)
		})
	})

	jobLog.Info("generation accepted",
		"target_keywords", len(job.cluster.Keywords),
	)

	return &server.GenerateAccepted{
		ID:             article.ID,
		Keyword:        article.Keyword,
		Status:         article.Status,
		CreatedAt:      article.CreatedAt,
		TargetKeywords: job.cluster.Keywords,
		SuggestedTitle: job.cluster.Title,
		Site:           job.site,
		Provider:       job.provider,
		Model:          job.model,
		StatusURL:      fmt.Sprintf("/articles/%d", article.ID),
	}, nil
}

// runWithRecover keeps a panic in any downstream layer from crashing the process.
func runWithRecover(log *slog.Logger, articleID int64, fn func()) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Error("background generation panic",
				"err", rec,
				"article_id", articleID,
				"stack", string(debug.Stack()),
			)
		}
	}()
	fn()
}

func (a *Application) runGeneration(ctx context.Context, log *slog.Logger, articleID int64, job resolvedJob, autoPublish bool) {
	out, err := a.generate(ctx, log, articleID, job)
	if err != nil {
		log.Error("generation failed", "err", err)
		// Fresh ctx: the job ctx is likely already cancelled, but the failed
		// status must still land in the DB.
		markCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if markErr := a.repo.MarkFailed(markCtx, articleID); markErr != nil {
			log.Error("mark article failed", "err", markErr)
		}
		return
	}

	if autoPublish {
		if !out.checkPassed {
			log.Warn("auto-publish blocked: originality check failed")
		} else if _, pubErr := a.publishArticle(ctx, articleID); pubErr != nil {
			log.Error("auto-publish failed", "err", pubErr)
		}
	}

	log.Info("generation persisted",
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
