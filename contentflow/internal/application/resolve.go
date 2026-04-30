package application

import (
	"context"
	"fmt"

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

	out, err := a.generate(ctx, articleID, job)
	if err != nil {
		a.log.Error("generation failed", "keyword", job.keyword, "err", err)
		if markErr := a.repo.MarkFailed(ctx, articleID); markErr != nil {
			a.log.Error("mark article failed", "article_id", articleID, "err", markErr)
		}
		return nil, err
	}

	// Optional auto-publish: flip the draft to published before returning.
	// Blocked when the originality check failed — the draft stays in WP for manual review.
	var autoPublishErr string
	if req.AutoPublish {
		if !out.checkPassed {
			autoPublishErr = "originality check failed — draft saved but not published"
			a.log.Warn("auto-publish blocked: originality check failed", "article_id", articleID)
		} else if _, pubErr := a.publishArticle(ctx, articleID); pubErr != nil {
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
		Content:          out.content,
		CreatedAt:        article.CreatedAt,
		UpdatedAt:        article.UpdatedAt,
		Provider:         out.provider,
		Model:            out.model,
		TargetKeywords:   job.cluster.Keywords,
		SuggestedTitle:   job.cluster.Title,
		CompetitorData:   out.competitorData,
		Humanized:        out.humanized,
		CheckCycles:      out.checkCycles,
		CheckResult:      out.checkResult,
		AutoPublishError: autoPublishErr,
		DurationMS:       out.durationMS,
		TokenUsage:       out.tokenSummary,
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

func pickFloat(req, def float64) float64 {
	if req != 0 {
		return req
	}
	return def
}
