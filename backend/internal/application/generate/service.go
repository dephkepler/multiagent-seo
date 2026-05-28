// Package generate is the application-layer use-case orchestrating the article
// generation pipeline over the generate domain ports. It owns no infrastructure:
// every dependency is a domain port injected via the constructor, and request
// defaults arrive as a config-agnostic Defaults struct.
package generate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/generate"
	"multiagent-seo/internal/domain/generate/prompt"
)

// ErrNoCluster signals the TopicSource returned no keywords for the topic; a
// missing cluster almost always means a typo or an un-seeded topic, so we abort
// before spending LLM tokens. The HTTP layer maps it to 404.
var ErrNoCluster = errors.New("no keyword cluster for topic")

// Publish guard errors mirror the legacy sentinels so the HTTP layer can map
// them to precise status codes.
var (
	ErrArticleNotFound  = errors.New("article not found")
	ErrAlreadyPublished = errors.New("article already published")
	ErrNoDraftToPublish = errors.New("article has no draft to publish")
)

// Defaults carries the per-request fallbacks ported from config. The Service
// takes these as a value struct (not pkg/config) so the application layer stays
// config-agnostic and unit-testable.
type Defaults struct {
	MinWords    int
	MaxWords    int
	Language    string
	Provider    string
	Model       string
	AIThreshold float64
	MaxCycles   int
	SERPLimit   int
	// SiteTopic and ExtraRules are the article-wide prompt defaults.
	SiteTopic  string
	ExtraRules string
	// IncludeImages is the default when the request omits it (true when an
	// ImageResolver is wired). A nil resolver still strips placeholders.
	IncludeImages bool
}

// GenerateRequest is the use-case input. Zero values fall back to Defaults.
// SiteID selects the WordPress site; the legacy free-form Site string is gone.
type GenerateRequest struct {
	Keyword     string
	SiteID      uuid.UUID
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
	// Pointers distinguish "omitted" from "explicit false".
	IncludeImages           *bool
	IncludeImageAttribution *bool
}

type Service struct {
	repo      generate.ArticleRepository
	llm       generate.LLMFactory
	serp      generate.SERPProvider
	topics    generate.TopicSource
	checker   generate.ContentChecker
	images    generate.ImageResolver
	publisher generate.PublisherProvider
	runner    JobRunner
	defaults  Defaults
	log       *slog.Logger
}

func NewService(
	repo generate.ArticleRepository,
	llm generate.LLMFactory,
	serp generate.SERPProvider,
	topics generate.TopicSource,
	checker generate.ContentChecker,
	images generate.ImageResolver,
	publisher generate.PublisherProvider,
	runner JobRunner,
	defaults Defaults,
	log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		repo:      repo,
		llm:       llm,
		serp:      serp,
		topics:    topics,
		checker:   checker,
		images:    images,
		publisher: publisher,
		runner:    runner,
		defaults:  defaults,
		log:       log,
	}
}

// spec is the per-request settings merged with defaults plus the resolved
// cluster and LLM client — the legacy resolvedJob without infra wiring.
type spec struct {
	keyword          string
	siteID           uuid.UUID
	minWords         int
	maxWords         int
	maxTokens        int
	language         string
	siteTopic        string
	extraRules       string
	cluster          generate.Cluster
	client           generate.LLMClient
	provider         string
	model            string
	maxCycles        int
	aiThreshold      float64
	includeImages    bool
	imageAttribution bool
	autoPublish      bool
}

// Generate resolves the spec, creates the article in "generating" state, then
// dispatches the pipeline through the JobRunner and returns immediately
// (202-style). A test using SyncRunner sees runGeneration complete first.
func (s *Service) Generate(ctx context.Context, req GenerateRequest) (generate.Article, error) {
	sp, err := s.resolve(ctx, req)
	if err != nil {
		return generate.Article{}, err
	}

	id, err := s.repo.Create(ctx, generate.CreateArticle{
		Keyword: sp.keyword,
		SiteID:  sp.siteID,
	})
	if err != nil {
		return generate.Article{}, fmt.Errorf("create article: %w", err)
	}

	article, err := s.repo.Get(ctx, id)
	if err != nil {
		return generate.Article{}, fmt.Errorf("fetch article: %w", err)
	}

	jobLog := s.log.With(
		"article_id", id,
		"keyword", sp.keyword,
		"site_id", sp.siteID,
		"provider", sp.provider,
		"model", sp.model,
	)

	s.runner.Go(ctx, func(bg context.Context) {
		s.runGeneration(bg, jobLog, id, sp)
	})

	jobLog.Info("generation accepted", "target_keywords", len(sp.cluster.Keywords))
	return *article, nil
}

func (s *Service) resolve(ctx context.Context, req GenerateRequest) (spec, error) {
	includeImages := s.defaults.IncludeImages
	if req.IncludeImages != nil {
		includeImages = *req.IncludeImages
	}
	attribution := true
	if req.IncludeImageAttribution != nil {
		attribution = *req.IncludeImageAttribution
	}

	sp := spec{
		keyword:          req.Keyword,
		siteID:           req.SiteID,
		minWords:         pickInt(req.MinWords, s.defaults.MinWords),
		maxWords:         pickInt(req.MaxWords, s.defaults.MaxWords),
		language:         pickStr(req.Language, s.defaults.Language),
		siteTopic:        pickStr(req.SiteTopic, s.defaults.SiteTopic),
		extraRules:       pickStr(req.ExtraRules, s.defaults.ExtraRules),
		provider:         pickStr(req.Provider, s.defaults.Provider),
		model:            pickStr(req.Model, s.defaults.Model),
		maxCycles:        pickInt(req.MaxCycles, s.defaults.MaxCycles),
		aiThreshold:      pickFloat(req.AIThreshold, s.defaults.AIThreshold),
		includeImages:    includeImages,
		imageAttribution: attribution,
		autoPublish:      req.AutoPublish,
	}

	cluster, err := s.topics.Lookup(ctx, req.Keyword)
	if err != nil {
		return spec{}, fmt.Errorf("cluster lookup: %w", err)
	}
	if len(cluster.Keywords) == 0 {
		s.log.Warn("no keyword cluster for topic", "keyword", req.Keyword)
		return spec{}, ErrNoCluster
	}
	sp.cluster = cluster

	// ~3 tokens/word + 200 overhead. 0 means "no cap" (Groq) / provider default.
	if req.MaxTokens > 0 {
		sp.maxTokens = req.MaxTokens
	} else if sp.maxWords > 0 {
		sp.maxTokens = sp.maxWords*3 + 200
	}

	client, err := s.llm.ForModel(sp.provider, sp.model)
	if err != nil {
		return spec{}, fmt.Errorf("build llm client: %w", err)
	}
	sp.client = client
	return sp, nil
}

// runGeneration runs the 5-step pipeline. Unexported and synchronous so a test
// can drive it through a SyncRunner. On any fatal step it marks the article
// failed via a fresh context, since the job context may already be cancelled.
func (s *Service) runGeneration(ctx context.Context, log *slog.Logger, articleID int64, sp spec) {
	checkPassed, err := s.pipeline(ctx, log, articleID, sp)
	if err != nil {
		log.Error("generation failed", "err", err)
		markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if markErr := s.repo.MarkFailed(markCtx, articleID); markErr != nil {
			log.Error("mark article failed", "err", markErr)
		}
		return
	}

	if sp.autoPublish {
		if !checkPassed {
			log.Warn("auto-publish blocked: originality check failed")
		} else if _, pubErr := s.Publish(ctx, articleID); pubErr != nil {
			log.Error("auto-publish failed", "err", pubErr)
		}
	}
}

// pipeline is the 5 steps; returns whether the originality check passed.
func (s *Service) pipeline(ctx context.Context, log *slog.Logger, articleID int64, sp spec) (bool, error) {
	// Step 1/5: SERP competitors. Non-fatal: a lookup error falls back to empty
	// competitor data and the article still generates.
	log.Info("step 1/5: serp competitors", "limit", s.defaults.SERPLimit)
	serpData, err := s.serp.GetSERP(ctx, sp.keyword, sp.language, s.defaults.SERPLimit)
	if err != nil {
		log.Warn("serp lookup failed, continuing without competitor data", "err", err)
		serpData = &generate.CompetitorData{Keyword: sp.keyword}
	} else {
		log.Info("serp competitors loaded", "count", len(serpData.Results))
		if saveErr := s.repo.SaveCompetitorData(ctx, articleID, serpData); saveErr != nil {
			log.Warn("save competitor data", "err", saveErr)
		}
	}
	competitors := prompt.CompetitorsFrom(serpData)

	// Step 2/5: brief.
	log.Info("step 2/5: brief", "competitors", len(competitors.Items), "target_keywords", len(sp.cluster.Keywords))
	brief, _, err := sp.client.Complete(ctx, prompt.Brief(sp.keyword, sp.language, sp.cluster, sp.siteTopic, sp.extraRules, competitors), sp.maxTokens)
	if err != nil {
		return false, fmt.Errorf("brief: %w", err)
	}

	// Step 3/5: writer.
	log.Info("step 3/5: writing", "min_words", sp.minWords, "max_words", sp.maxWords)
	article, _, err := sp.client.Complete(ctx, prompt.Writer(brief, sp.keyword, sp.language, sp.minWords, sp.maxWords, sp.cluster, competitors), sp.maxTokens)
	if err != nil {
		return false, fmt.Errorf("writer: %w", err)
	}

	// Step 4/5: editor.
	log.Info("step 4/5: editing")
	edited, _, err := sp.client.Complete(ctx, prompt.Editor(article, sp.keyword, sp.minWords, sp.maxWords, sp.cluster, competitors), sp.maxTokens)
	if err != nil {
		return false, fmt.Errorf("editor: %w", err)
	}

	// Step 5/5: originality check + humanize loop.
	log.Info("step 5/5: originality check")
	edited, lastCheck := s.checkAndHumanize(ctx, log, articleID, sp, edited)

	cleaned, seoTitle, seoDesc := extractSEOFields(edited)
	if seoTitle != "" || seoDesc != "" {
		log.Info("seo fields extracted", "seo_title", seoTitle, "seo_desc", seoDesc)
	}

	// A nil resolver makes RenderHTML strip [IMG] placeholders, mirroring the
	// legacy pexels-nil behaviour.
	var resolver generate.ImageResolver
	if sp.includeImages {
		resolver = s.images
	}
	body, renderStats := generate.RenderHTML(ctx, cleaned, generate.RenderOptions{
		Keyword:     sp.keyword,
		Resolver:    resolver,
		Attribution: sp.imageAttribution,
	})

	pub, err := s.publisher.ForSite(ctx, sp.siteID)
	if err != nil {
		return false, fmt.Errorf("resolve publisher: %w", err)
	}

	postID, editURL, err := pub.CreateDraft(ctx, generate.Post{
		Title:    sp.keyword,
		Content:  body,
		SEOTitle: seoTitle,
		SEODesc:  seoDesc,
	})
	if err != nil {
		return false, fmt.Errorf("publisher draft: %w", err)
	}

	if err := s.repo.UpdateDraft(ctx, articleID, postID, editURL); err != nil {
		return false, fmt.Errorf("update draft: %w", err)
	}
	if err := s.repo.SaveImageStats(ctx, articleID, renderStats.ImagesRequested, renderStats.ImagesResolved, renderStats.ImagesSkipped); err != nil {
		log.Warn("save image stats", "err", err)
	}

	checkPassed := lastCheck == nil || lastCheck.Original
	log.Info("generation done",
		"post_id", postID,
		"check_passed", checkPassed,
		"images_requested", renderStats.ImagesRequested,
		"images_resolved", renderStats.ImagesResolved,
		"images_skipped", renderStats.ImagesSkipped,
	)
	return checkPassed, nil
}

// checkAndHumanize runs the check→humanize loop up to maxCycles, persisting each
// CheckResult. Returns the (possibly rewritten) content and the last check.
func (s *Service) checkAndHumanize(ctx context.Context, log *slog.Logger, articleID int64, sp spec, content string) (string, *generate.CheckResult) {
	maxCycles := sp.maxCycles
	if maxCycles <= 0 {
		maxCycles = 3
	}
	threshold := sp.aiThreshold
	if threshold == 0 {
		threshold = s.defaults.AIThreshold
	}

	var last *generate.CheckResult

	for cycle := 1; cycle <= maxCycles; cycle++ {
		checkRes, err := s.checker.Check(ctx, content)
		if err != nil {
			log.Warn("originality check failed, skipping", "cycle", cycle, "err", err)
			break
		}

		passes := checkRes.AIScore < threshold
		checkRes.Original = passes
		last = checkRes

		log.Info("check result", "cycle", cycle, "ai_score", checkRes.AIScore, "threshold", threshold, "passes", passes)

		if saveErr := s.repo.SaveCheckResult(ctx, articleID, checkRes); saveErr != nil {
			log.Warn("save check result", "err", saveErr)
		}

		if passes {
			return content, last
		}
		if cycle == maxCycles {
			log.Warn("max humanize cycles reached, publishing as-is", "cycles", maxCycles, "final_ai_score", checkRes.AIScore)
			break
		}

		log.Info("content flagged — humanize rewrite", "cycle", cycle, "sentences_flagged", len(checkRes.SentencesFlagged))
		rewritten, _, err := sp.client.Complete(ctx, prompt.Humanize(content, sp.keyword, checkRes.Issues, checkRes.SentencesFlagged), sp.maxTokens)
		if err != nil {
			log.Warn("humanize step failed, using current content", "cycle", cycle, "err", err)
			break
		}
		content = rewritten
	}

	return content, last
}

// Publish promotes an existing draft live. Mirrors the legacy publishArticle
// status guards and maps domain errors to the use-case sentinels.
func (s *Service) Publish(ctx context.Context, articleID int64) (generate.Article, error) {
	article, err := s.repo.Get(ctx, articleID)
	if err != nil {
		if errors.Is(err, generate.ErrNotFound) {
			return generate.Article{}, ErrArticleNotFound
		}
		return generate.Article{}, fmt.Errorf("fetch article: %w", err)
	}

	switch article.Status {
	case generate.StatusPublished:
		return generate.Article{}, ErrAlreadyPublished
	case generate.StatusGenerating, generate.StatusFailed:
		return generate.Article{}, ErrNoDraftToPublish
	}
	if article.WPPostID == 0 {
		return generate.Article{}, ErrNoDraftToPublish
	}

	pub, err := s.publisher.ForSite(ctx, article.SiteID)
	if err != nil {
		return generate.Article{}, fmt.Errorf("resolve publisher: %w", err)
	}

	postURL, err := pub.Publish(ctx, article.WPPostID)
	if err != nil {
		return generate.Article{}, fmt.Errorf("publish: %w", err)
	}

	if err := s.repo.MarkPublished(ctx, articleID, postURL); err != nil {
		return generate.Article{}, fmt.Errorf("mark published: %w", err)
	}

	updated, err := s.repo.Get(ctx, articleID)
	if err != nil {
		return generate.Article{}, fmt.Errorf("fetch article after publish: %w", err)
	}
	s.log.Info("article published", "article_id", articleID, "wp_post_id", updated.WPPostID, "wp_post_url", updated.WPPostURL)
	return *updated, nil
}

// List returns all tracked articles for the read API.
func (s *Service) List(ctx context.Context) ([]generate.Article, error) {
	arts, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list articles: %w", err)
	}
	return arts, nil
}

// Get fetches one article, mapping the domain not-found to the use-case sentinel.
func (s *Service) Get(ctx context.Context, id int64) (generate.Article, error) {
	article, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, generate.ErrNotFound) {
			return generate.Article{}, ErrArticleNotFound
		}
		return generate.Article{}, fmt.Errorf("fetch article: %w", err)
	}
	return *article, nil
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
