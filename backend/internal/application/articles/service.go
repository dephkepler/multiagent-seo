package articles

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/domain/articles/prompt"
	"multiagent-seo/pkg/jobrunner"
)

var ErrNoCluster = errors.New("no keyword cluster for topic")

var (
	ErrArticleNotFound  = errors.New("article not found")
	ErrAlreadyPublished = errors.New("article already published")
	ErrNoDraftToPublish = errors.New("article has no draft to publish")
)

type Defaults struct {
	MinWords      int
	MaxWords      int
	Language      string
	Provider      string
	Model         string
	AIThreshold   float64
	MaxCycles     int
	SERPLimit     int
	SiteTopic     string
	ExtraRules    string
	IncludeImages bool
}

type GenerateRequest struct {
	Keyword                 string
	SiteID                  uuid.UUID
	MinWords                int
	MaxWords                int
	MaxTokens               int
	Language                string
	SiteTopic               string
	ExtraRules              string
	Provider                string
	Model                   string
	AutoPublish             bool
	MaxCycles               int
	AIThreshold             float64
	IncludeImages           *bool
	IncludeImageAttribution *bool
}

type Service struct {
	repo      articles.ArticleRepository
	llm       articles.LLMFactory
	serp      articles.SERPProvider
	topics    articles.TopicSource
	checker   articles.ContentChecker
	images    articles.ImageResolver
	publisher articles.PublisherProvider
	runner    jobrunner.JobRunner
	defaults  Defaults
	log       *slog.Logger
}

func NewService(
	repo articles.ArticleRepository,
	llm articles.LLMFactory,
	serp articles.SERPProvider,
	topics articles.TopicSource,
	checker articles.ContentChecker,
	images articles.ImageResolver,
	publisher articles.PublisherProvider,
	runner jobrunner.JobRunner,
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

type spec struct {
	keyword          string
	siteID           uuid.UUID
	minWords         int
	maxWords         int
	maxTokens        int
	language         string
	siteTopic        string
	extraRules       string
	cluster          articles.Cluster
	client           articles.LLMClient
	provider         string
	model            string
	maxCycles        int
	aiThreshold      float64
	includeImages    bool
	imageAttribution bool
	autoPublish      bool
}

type GenerateResult struct {
	Article        articles.Article
	TargetKeywords []string
	SuggestedTitle string
	Provider       string
	Model          string
}

func (s *Service) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	settings, err := s.resolve(ctx, req)
	if err != nil {
		return GenerateResult{}, err
	}

	id, err := s.repo.Create(ctx, articles.CreateArticle{
		Keyword: settings.keyword,
		SiteID:  settings.siteID,
	})
	if err != nil {
		return GenerateResult{}, fmt.Errorf("create article: %w", err)
	}

	article, err := s.repo.Get(ctx, id)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("fetch article: %w", err)
	}

	jobLog := s.log.With(
		"article_id", id,
		"keyword", settings.keyword,
		"site_id", settings.siteID,
		"provider", settings.provider,
		"model", settings.model,
	)

	s.runner.Go(ctx, func(bg context.Context) {
		s.runGeneration(bg, jobLog, id, settings)
	})

	jobLog.InfoContext(ctx, "generation accepted", "target_keywords", len(settings.cluster.Keywords))
	return GenerateResult{
		Article:        *article,
		TargetKeywords: settings.cluster.Keywords,
		SuggestedTitle: settings.cluster.Title,
		Provider:       settings.provider,
		Model:          settings.model,
	}, nil
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

	provider := pickStr(req.Provider, s.defaults.Provider)
	model := req.Model
	if model == "" && provider == s.defaults.Provider {
		model = s.defaults.Model
	}

	settings := spec{
		keyword:          req.Keyword,
		siteID:           req.SiteID,
		minWords:         pickInt(req.MinWords, s.defaults.MinWords),
		maxWords:         pickInt(req.MaxWords, s.defaults.MaxWords),
		language:         pickStr(req.Language, s.defaults.Language),
		siteTopic:        pickStr(req.SiteTopic, s.defaults.SiteTopic),
		extraRules:       pickStr(req.ExtraRules, s.defaults.ExtraRules),
		provider:         provider,
		model:            model,
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
		s.log.WarnContext(ctx, "no keyword cluster for topic", "keyword", req.Keyword)
		return spec{}, ErrNoCluster
	}
	settings.cluster = cluster

	if req.MaxTokens > 0 {
		settings.maxTokens = req.MaxTokens
	} else if settings.maxWords > 0 {
		settings.maxTokens = settings.maxWords*3 + 200
	}

	client, err := s.llm.ForModel(settings.provider, settings.model)
	if err != nil {
		return spec{}, fmt.Errorf("build llm client: %w", err)
	}
	settings.client = client
	return settings, nil
}

func (s *Service) runGeneration(ctx context.Context, log *slog.Logger, articleID int64, settings spec) {
	checkPassed, err := s.pipeline(ctx, log, articleID, settings)
	if err != nil {
		log.ErrorContext(ctx, "generation failed", "err", err)
		markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if markErr := s.repo.MarkFailed(markCtx, articleID); markErr != nil {
			log.ErrorContext(ctx, "mark article failed", "err", markErr)
		}
		return
	}

	if settings.autoPublish {
		if !checkPassed {
			log.WarnContext(ctx, "auto-publish blocked: originality check failed")
		} else {
			_, _ = s.Publish(ctx, articleID)
		}
	}
}

func (s *Service) pipeline(ctx context.Context, log *slog.Logger, articleID int64, settings spec) (bool, error) {
	log.DebugContext(ctx, "step 1/5: serp competitors", "limit", s.defaults.SERPLimit)
	serpData, err := s.serp.GetSERP(ctx, settings.keyword, settings.language, s.defaults.SERPLimit)
	if err != nil {
		log.WarnContext(ctx, "serp lookup failed, continuing without competitor data", "err", err)
		serpData = &articles.CompetitorData{Keyword: settings.keyword}
	} else {
		log.DebugContext(ctx, "serp competitors loaded", "count", len(serpData.Results))
		if saveErr := s.repo.SaveCompetitorData(ctx, articleID, serpData); saveErr != nil {
			log.WarnContext(ctx, "save competitor data", "err", saveErr)
		}
	}
	competitors := prompt.CompetitorsFrom(serpData)

	log.DebugContext(ctx, "step 2/5: brief", "competitors", len(competitors.Items), "target_keywords", len(settings.cluster.Keywords))
	brief, _, err := settings.client.Complete(ctx, prompt.Brief(settings.keyword, settings.language, settings.cluster, settings.siteTopic, settings.extraRules, competitors), settings.maxTokens)
	if err != nil {
		return false, fmt.Errorf("brief: %w", err)
	}

	log.DebugContext(ctx, "step 3/5: writing", "min_words", settings.minWords, "max_words", settings.maxWords)
	article, _, err := settings.client.Complete(ctx, prompt.Writer(brief, settings.keyword, settings.language, settings.minWords, settings.maxWords, settings.cluster, competitors), settings.maxTokens)
	if err != nil {
		return false, fmt.Errorf("writer: %w", err)
	}

	log.DebugContext(ctx, "step 4/5: editing")
	edited, _, err := settings.client.Complete(ctx, prompt.Editor(article, settings.keyword, settings.minWords, settings.maxWords, settings.cluster, competitors), settings.maxTokens)
	if err != nil {
		return false, fmt.Errorf("editor: %w", err)
	}

	log.DebugContext(ctx, "step 5/5: originality check")
	edited, lastCheck := s.checkAndHumanize(ctx, log, articleID, settings, edited)

	cleaned, seoTitle, seoDesc := extractSEOFields(edited)
	if seoTitle != "" || seoDesc != "" {
		log.DebugContext(ctx, "seo fields extracted", "seo_title", seoTitle, "seo_desc", seoDesc)
	} else {
		log.DebugContext(ctx, "no seo fields extracted")
	}

	var resolver articles.ImageResolver
	if settings.includeImages {
		resolver = s.images
	}
	body, renderStats := articles.RenderHTML(ctx, cleaned, articles.RenderOptions{
		Keyword:     settings.keyword,
		Resolver:    resolver,
		Attribution: settings.imageAttribution,
	})
	if renderStats.ImagesFailed > 0 {
		log.WarnContext(ctx, "image resolution failed for some placeholders",
			"images_failed", renderStats.ImagesFailed,
			"images_requested", renderStats.ImagesRequested,
		)
	}

	pub, err := s.publisher.ForSite(ctx, settings.siteID)
	if err != nil {
		return false, fmt.Errorf("resolve publisher: %w", err)
	}

	postID, editURL, err := pub.CreateDraft(ctx, articles.Post{
		Title:    settings.keyword,
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
		log.WarnContext(ctx, "save image stats", "err", err)
	}

	checkPassed := lastCheck == nil || lastCheck.Original
	log.InfoContext(ctx, "generation done",
		"post_id", postID,
		"check_passed", checkPassed,
		"images_requested", renderStats.ImagesRequested,
		"images_resolved", renderStats.ImagesResolved,
		"images_skipped", renderStats.ImagesSkipped,
	)
	return checkPassed, nil
}

func (s *Service) checkAndHumanize(ctx context.Context, log *slog.Logger, articleID int64, settings spec, content string) (string, *articles.CheckResult) {
	maxCycles := settings.maxCycles
	if maxCycles <= 0 {
		maxCycles = 3
	}
	threshold := settings.aiThreshold
	if threshold == 0 {
		threshold = s.defaults.AIThreshold
	}
	if threshold <= 0 {
		threshold = 0.8
	}

	var last *articles.CheckResult

	for cycle := 1; cycle <= maxCycles; cycle++ {
		checkRes, err := s.checker.Check(ctx, content)
		if err != nil {
			log.WarnContext(ctx, "originality check failed, skipping", "cycle", cycle, "err", err)
			break
		}

		passes := checkRes.AIScore < threshold
		checkRes.Original = passes
		last = checkRes

		log.DebugContext(ctx, "check result", "cycle", cycle, "ai_score", checkRes.AIScore, "threshold", threshold, "passes", passes)

		if saveErr := s.repo.SaveCheckResult(ctx, articleID, checkRes); saveErr != nil {
			log.WarnContext(ctx, "save check result", "err", saveErr)
		}

		if passes {
			return content, last
		}
		if cycle == maxCycles {
			log.WarnContext(ctx, "max humanize cycles reached, publishing as-is", "cycles", maxCycles, "final_ai_score", checkRes.AIScore)
			break
		}

		log.DebugContext(ctx, "content flagged — humanize rewrite", "cycle", cycle, "sentences_flagged", len(checkRes.SentencesFlagged))
		rewritten, _, err := settings.client.Complete(ctx, prompt.Humanize(content, settings.keyword, checkRes.Issues, checkRes.SentencesFlagged), settings.maxTokens)
		if err != nil {
			log.WarnContext(ctx, "humanize step failed, using current content", "cycle", cycle, "err", err)
			break
		}
		content = rewritten
	}

	return content, last
}

func (s *Service) Publish(ctx context.Context, articleID int64) (articles.Article, error) {
	article, err := s.repo.Get(ctx, articleID)
	if err != nil {
		if errors.Is(err, articles.ErrNotFound) {
			return articles.Article{}, ErrArticleNotFound
		}
		return articles.Article{}, fmt.Errorf("fetch article: %w", err)
	}

	switch article.Status {
	case articles.StatusPublished:
		return articles.Article{}, ErrAlreadyPublished
	case articles.StatusGenerating, articles.StatusFailed:
		return articles.Article{}, ErrNoDraftToPublish
	}
	if article.WPPostID == 0 {
		return articles.Article{}, ErrNoDraftToPublish
	}

	pub, err := s.publisher.ForSite(ctx, article.SiteID)
	if err != nil {
		s.log.ErrorContext(ctx, "publish failed", "article_id", articleID, "stage", "resolve publisher", "err", err)
		return articles.Article{}, fmt.Errorf("resolve publisher: %w", err)
	}

	postURL, err := pub.Publish(ctx, article.WPPostID)
	if err != nil {
		s.log.ErrorContext(ctx, "publish failed", "article_id", articleID, "wp_post_id", article.WPPostID, "stage", "publish", "err", err)
		return articles.Article{}, fmt.Errorf("publish: %w", err)
	}

	if err := s.repo.MarkPublished(ctx, articleID, postURL); err != nil {
		s.log.ErrorContext(ctx, "publish failed", "article_id", articleID, "stage", "mark published", "err", err)
		return articles.Article{}, fmt.Errorf("mark published: %w", err)
	}

	updated, err := s.repo.Get(ctx, articleID)
	if err != nil {
		return articles.Article{}, fmt.Errorf("fetch article after publish: %w", err)
	}
	s.log.InfoContext(ctx, "article published", "article_id", articleID, "wp_post_id", updated.WPPostID, "wp_post_url", updated.WPPostURL)
	return *updated, nil
}

func (s *Service) List(ctx context.Context) ([]articles.Article, error) {
	arts, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list articles: %w", err)
	}
	return arts, nil
}

func (s *Service) Get(ctx context.Context, id int64) (articles.Article, error) {
	article, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, articles.ErrNotFound) {
			return articles.Article{}, ErrArticleNotFound
		}
		return articles.Article{}, fmt.Errorf("fetch article: %w", err)
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
