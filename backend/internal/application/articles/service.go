package articles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/domain/articles/prompt"
	"multiagent-seo/pkg/jobrunner"
)

var (
	ErrNoCluster        = errors.New("no keyword cluster for topic")
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
	HumanWeight   float64
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
	cluster                 *articles.Cluster
}

type Service struct {
	repo      articles.ArticleRepository
	llm       articles.LLMFactory
	serp      articles.SERPProvider
	topics    articles.TopicSourceProvider
	checker   articles.ContentChecker
	images    articles.ImageResolver
	publisher articles.PublisherProvider
	prompts   articles.PromptStore
	runner    jobrunner.JobRunner
	defaults  Defaults
	log       *slog.Logger
}

func NewService(
	repo articles.ArticleRepository,
	llm articles.LLMFactory,
	serp articles.SERPProvider,
	topics articles.TopicSourceProvider,
	checker articles.ContentChecker,
	images articles.ImageResolver,
	publisher articles.PublisherProvider,
	prompts articles.PromptStore,
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
		prompts:   prompts,
		runner:    runner,
		defaults:  defaults,
		log:       log,
	}
}

type genSettings struct {
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

type articleRequestParams struct {
	MinWords      int     `json:"min_words"`
	MaxWords      int     `json:"max_words"`
	MaxTokens     int     `json:"max_tokens"`
	MaxCycles     int     `json:"max_cycles"`
	AIThreshold   float64 `json:"ai_threshold"`
	Provider      string  `json:"provider"`
	Model         string  `json:"model"`
	Language      string  `json:"language"`
	AutoPublish   bool    `json:"auto_publish"`
	IncludeImages bool    `json:"include_images"`
}

func (s *Service) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	settings, err := s.resolve(ctx, req)
	if err != nil {
		if errors.Is(err, ErrNoCluster) {
			return GenerateResult{}, err
		}
		return GenerateResult{}, fmt.Errorf("resolve generation settings (keyword=%q site_id=%s): %w", req.Keyword, req.SiteID, err)
	}

	article, err := s.createArticle(ctx, settings)
	if err != nil {
		return GenerateResult{}, err
	}
	id := article.ID

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
		Article:        article,
		TargetKeywords: settings.cluster.Keywords,
		SuggestedTitle: settings.cluster.Title,
		Provider:       settings.provider,
		Model:          settings.model,
	}, nil
}

func (s *Service) createArticle(ctx context.Context, settings genSettings) (articles.Article, error) {
	reqParams, err := json.Marshal(articleRequestParams{
		MinWords:      settings.minWords,
		MaxWords:      settings.maxWords,
		MaxTokens:     settings.maxTokens,
		MaxCycles:     settings.maxCycles,
		AIThreshold:   settings.aiThreshold,
		Provider:      settings.provider,
		Model:         settings.model,
		Language:      settings.language,
		AutoPublish:   settings.autoPublish,
		IncludeImages: settings.includeImages,
	})
	if err != nil {
		return articles.Article{}, fmt.Errorf("marshal request params: %w", err)
	}

	article, err := s.repo.Create(ctx, articles.CreateArticle{
		Keyword:       settings.keyword,
		SiteID:        settings.siteID,
		RequestParams: reqParams,
	})
	if err != nil {
		return articles.Article{}, err
	}
	return article, nil
}

func (s *Service) resolve(ctx context.Context, req GenerateRequest) (genSettings, error) {
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

	settings := genSettings{
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

	cluster := articles.Cluster{}
	if req.cluster != nil {
		cluster = *req.cluster
	} else {
		topics, err := s.topics.ForSite(ctx, req.SiteID, settings.language)
		if err != nil {
			return genSettings{}, fmt.Errorf("topic source for site %s: %w", req.SiteID, err)
		}
		c, err := topics.Lookup(ctx, req.Keyword)
		if err != nil {
			return genSettings{}, fmt.Errorf("cluster lookup: %w", err)
		}
		cluster = c
	}
	if len(cluster.Keywords) == 0 {
		return genSettings{}, ErrNoCluster
	}
	settings.cluster = cluster

	if req.MaxTokens > 0 {
		settings.maxTokens = req.MaxTokens
	} else if settings.maxWords > 0 {
		settings.maxTokens = estimateMaxTokens(settings.maxWords)
	}

	client, err := s.llm.ForModel(settings.provider, settings.model)
	if err != nil {
		return genSettings{}, fmt.Errorf("build llm client: %w", err)
	}
	settings.client = client
	return settings, nil
}

func (s *Service) runGeneration(ctx context.Context, log *slog.Logger, articleID int64, settings genSettings) {
	defer func() {
		if rec := recover(); rec != nil {
			log.ErrorContext(ctx, "generation panic", "err", rec, "stack", string(debug.Stack()))
			s.markFailed(ctx, log, articleID)
		}
	}()

	checkPassed, err := s.pipeline(ctx, log, articleID, settings)
	if err != nil {
		log.ErrorContext(ctx, "generation failed", "err", err)
		s.markFailed(ctx, log, articleID)
		return
	}

	if settings.autoPublish {
		if !checkPassed {
			log.DebugContext(ctx, "auto-publish skipped: originality check did not pass", "article_id", articleID)
		} else if _, err := s.publish(ctx, log, articleID); err != nil {
			log.WarnContext(ctx, "auto-publish failed", "article_id", articleID, "err", err)
			s.markFailed(ctx, log, articleID)
		}
	}
}

func (s *Service) markFailed(ctx context.Context, log *slog.Logger, articleID int64) {
	markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.repo.MarkFailed(markCtx, articleID); err != nil {
		log.ErrorContext(ctx, "mark article failed", "err", err)
	}
}

func (s *Service) stageTimer(ctx context.Context,
	articleID int64,
	stage,
	provider,
	model string,
) func(articles.Usage, bool) {
	start := time.Now()
	return func(u articles.Usage, ok bool) {
		if err := s.repo.SaveEvent(ctx, articles.GenerationEvent{
			ArticleID:    articleID,
			Stage:        stage,
			Provider:     provider,
			Model:        model,
			InputTokens:  u.InputTokens,
			OutputTokens: u.OutputTokens,
			LatencyMS:    time.Since(start).Milliseconds(),
			OK:           ok,
		}); err != nil {
			s.log.DebugContext(ctx, "save generation event failed, continuing", "err", err, "stage", stage)
		}
	}
}

const writerSelectionWindow = 30 * 24 * time.Hour

func (s *Service) writerTemplate(ctx context.Context) (body string, variantID int64) {
	if s.prompts == nil {
		return prompt.WriterTemplate, 0
	}
	stats, err := s.prompts.SelectionStats(ctx, articles.PromptStageWriter, time.Now().Add(-writerSelectionWindow))
	if err != nil {
		s.log.DebugContext(ctx, "load writer prompt stats, using built-in", "err", err)
		return prompt.WriterTemplate, 0
	}
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	pick, ok := articles.ThompsonPick(rng, stats)
	if !ok {
		return prompt.WriterTemplate, 0
	}
	return pick.Body, pick.ID
}

const maxBatchCount = 100

func (s *Service) GenerateBatch(ctx context.Context, count int, shared GenerateRequest) ([]int64, error) {
	if count <= 0 {
		return nil, nil
	}
	if count > maxBatchCount {
		count = maxBatchCount
	}

	language := pickStr(shared.Language, s.defaults.Language)
	topicSource, err := s.topics.ForSite(ctx, shared.SiteID, language)
	if err != nil {
		return nil, fmt.Errorf("topic source for site %s: %w", shared.SiteID, err)
	}
	topics, err := topicSource.Topics(ctx)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	clusters, err := topicSource.Clusters(ctx)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	done, err := s.repo.GeneratedKeywords(ctx, shared.SiteID)
	if err != nil {
		return nil, fmt.Errorf("existing keywords: %w", err)
	}
	doneSet := make(map[string]bool, len(done))
	for _, k := range done {
		doneSet[strings.ToLower(strings.TrimSpace(k))] = true
	}

	ids := make([]int64, 0, count)
	for _, t := range topics {
		if len(ids) >= count {
			break
		}
		key := strings.ToLower(strings.TrimSpace(t))
		if doneSet[key] {
			continue
		}
		req := shared
		req.Keyword = t
		if c, ok := clusters[key]; ok {
			req.cluster = &c
		}
		res, err := s.Generate(ctx, req)
		if err != nil {
			s.log.WarnContext(ctx, "batch: skip topic", "topic", t, "err", err)
			continue
		}
		ids = append(ids, res.Article.ID)
	}
	if len(ids) < count {
		s.log.InfoContext(ctx, "batch: fewer articles queued than requested",
			"requested", count, "queued", len(ids), "topics_available", len(topics))
	}
	return ids, nil
}

func (s *Service) RateArticle(ctx context.Context, articleID int64, rating *bool) error {
	if err := s.repo.SetHumanRating(ctx, articleID, rating); err != nil {
		return fmt.Errorf("set human rating: %w", err)
	}
	if s.prompts == nil {
		return nil
	}

	var human *float64
	if rating != nil {
		v := 0.0
		if *rating {
			v = 1.0
		}
		human = &v
	}
	weight := s.defaults.HumanWeight
	if err := s.prompts.UpdateOutcomeReward(ctx, articleID, articles.PromptStageWriter, human, weight); err != nil {
		return fmt.Errorf("update outcome reward: %w", err)
	}
	return nil
}

func (s *Service) recordWriterOutcome(
	ctx context.Context,
	log *slog.Logger,
	articleID, variantID int64,
	check checkOutcome,
	settings genSettings,
	cleaned, seoTitle, seoDesc string,
	tokens int,
) {
	if s.prompts == nil {
		return
	}
	ok, reasons := articles.QualityFloor(articles.QualityInput{
		Content:        cleaned,
		SEOTitle:       seoTitle,
		SEODescription: seoDesc,
		Keywords:       settings.cluster.Keywords,
		MinWords:       settings.minWords,
		MaxWords:       settings.maxWords,
	})
	if !ok {
		log.DebugContext(ctx, "quality floor failed", "reasons", reasons)
	}
	var vid *int64
	if variantID != 0 {
		vid = &variantID
	}
	var reward *float64
	if check.firstScore != nil {
		r := 1 - *check.firstScore
		reward = &r
	}
	if err := s.prompts.SaveOutcome(ctx, articles.PromptOutcome{
		ArticleID:      articleID,
		Stage:          articles.PromptStageWriter,
		VariantID:      vid,
		Reward:         reward,
		AIScore:        check.firstScore,
		QualityOK:      ok,
		HumanizeCycles: check.humanizeCycles,
		Tokens:         tokens,
	}); err != nil {
		log.DebugContext(ctx, "save prompt outcome failed, continuing", "err", err)
	}
}

func (s *Service) pipeline(ctx context.Context, log *slog.Logger, articleID int64, settings genSettings) (bool, error) {
	content, err := s.generateContent(ctx, log, articleID, settings)
	if err != nil {
		return false, err
	}
	return s.finalizeDraft(ctx, log, articleID, settings, content)
}

type generated struct {
	check         checkOutcome
	writerVariant int64
	tokens        int
}

func (s *Service) generateContent(ctx context.Context, log *slog.Logger, articleID int64, settings genSettings) (generated, error) {
	log.DebugContext(ctx, "step 1/5: serp competitors", "limit", s.defaults.SERPLimit)
	serpDone := s.stageTimer(
		ctx,
		articleID,
		articles.StageSERP,
		"dataforseo",
		"",
	)
	serpData, err := s.serp.GetSERP(ctx, settings.keyword, settings.language, s.defaults.SERPLimit)
	serpDone(articles.Usage{}, err == nil)
	if err != nil {
		log.DebugContext(ctx, "serp lookup failed, continuing without competitor data", "err", err)
		serpData = &articles.CompetitorData{Keyword: settings.keyword}
	} else {
		log.DebugContext(ctx, "serp competitors loaded", "count", len(serpData.Results))
		if saveErr := s.repo.SaveCompetitorData(ctx, articleID, serpData); saveErr != nil {
			log.DebugContext(ctx, "save competitor data failed, continuing", "err", saveErr)
		}
	}
	competitors := prompt.CompetitorsFrom(serpData)

	log.DebugContext(ctx, "step 2/5: brief", "competitors", len(competitors.Items), "target_keywords", len(settings.cluster.Keywords))
	brief, briefUsage, err := s.runLLMStage(
		ctx,
		articleID,
		articles.StageBrief,
		"brief",
		prompt.Brief(settings.keyword, settings.language, settings.cluster, settings.siteTopic, settings.extraRules, competitors),
		settings,
	)
	if err != nil {
		return generated{}, err
	}

	log.DebugContext(ctx, "step 3/5: writing", "min_words", settings.minWords, "max_words", settings.maxWords)
	writerBody, writerVariant := s.writerTemplate(ctx)
	article, writeUsage, err := s.runLLMStage(
		ctx,
		articleID,
		articles.StageWrite,
		"writer",
		prompt.RenderWriter(writerBody, brief, settings.keyword, settings.language, settings.minWords, settings.maxWords, settings.cluster, competitors),
		settings,
	)
	if err != nil {
		return generated{}, err
	}

	log.DebugContext(ctx, "step 4/5: editing")
	edited, editUsage, err := s.runLLMStage(
		ctx,
		articleID,
		articles.StageEdit,
		"editor",
		prompt.Editor(article, settings.keyword, settings.minWords, settings.maxWords, settings.cluster, competitors),
		settings,
	)
	if err != nil {
		return generated{}, err
	}

	log.DebugContext(ctx, "step 5/5: originality check")
	check := s.checkAndHumanize(ctx, log, articleID, settings, edited)

	tokens := briefUsage.Total() + writeUsage.Total() + editUsage.Total() + check.humanizeTokens
	return generated{check: check, writerVariant: writerVariant, tokens: tokens}, nil
}

func (s *Service) finalizeDraft(ctx context.Context, log *slog.Logger, articleID int64, settings genSettings, content generated) (bool, error) {
	cleaned, seoTitle, seoDesc := extractSEOFields(content.check.content)

	var resolver articles.ImageResolver
	if settings.includeImages {
		resolver = s.images
	}
	imagesDone := s.stageTimer(
		ctx,
		articleID,
		articles.StageImages,
		"pexels",
		"",
	)
	body, renderStats, err := articles.RenderHTML(ctx, cleaned, articles.RenderOptions{
		Keyword:     settings.keyword,
		Resolver:    resolver,
		Attribution: settings.imageAttribution,
		Log:         log,
	})
	if err != nil {
		return false, fmt.Errorf("render html: %w", err)
	}
	if settings.includeImages && renderStats.ImagesRequested > 0 {
		imagesDone(articles.Usage{}, renderStats.ImagesFailed == 0)
	}
	if renderStats.ImagesFailed > 0 {
		log.InfoContext(ctx, "some image placeholders could not be resolved",
			"images_failed", renderStats.ImagesFailed,
			"images_requested", renderStats.ImagesRequested,
		)
	}

	if _, revErr := s.repo.SaveRevision(ctx, articles.Revision{
		ArticleID:      articleID,
		Source:         articles.RevisionGenerated,
		ContentMD:      cleaned,
		ContentHTML:    body,
		SEOTitle:       seoTitle,
		SEODescription: seoDesc,
		WordCount:      len(strings.Fields(cleaned)),
	}); revErr != nil {
		log.DebugContext(ctx, "save revision failed, continuing", "err", revErr)
	}

	s.recordWriterOutcome(
		ctx,
		log,
		articleID,
		content.writerVariant,
		content.check,
		settings,
		cleaned,
		seoTitle,
		seoDesc,
		content.tokens,
	)

	pub, err := s.publisher.ForSite(ctx, settings.siteID, settings.language)
	if err != nil {
		return false, fmt.Errorf("resolve publisher: %w", err)
	}

	draftDone := s.stageTimer(
		ctx,
		articleID,
		articles.StageDraft,
		"wordpress",
		"",
	)
	postID, editURL, err := pub.CreateDraft(ctx, articles.Post{
		Title:    settings.keyword,
		Content:  body,
		SEOTitle: seoTitle,
		SEODesc:  seoDesc,
	})
	draftDone(articles.Usage{}, err == nil)
	if err != nil {
		return false, fmt.Errorf("publisher draft: %w", err)
	}

	if err := s.repo.UpdateDraft(ctx, articleID, postID, editURL); err != nil {
		return false, fmt.Errorf("update draft: %w", err)
	}
	if err := s.repo.SaveImageStats(
		ctx,
		articleID,
		renderStats.ImagesRequested,
		renderStats.ImagesResolved,
		renderStats.ImagesSkipped,
	); err != nil {
		log.DebugContext(ctx, "save image stats failed, continuing", "err", err)
	}

	checkPassed := content.check.passed()
	log.InfoContext(ctx, "generation done",
		"post_id", postID,
		"check_passed", checkPassed,
		"images_requested", renderStats.ImagesRequested,
		"images_resolved", renderStats.ImagesResolved,
		"images_skipped", renderStats.ImagesSkipped,
	)
	return checkPassed, nil
}

func (s *Service) runLLMStage(
	ctx context.Context,
	articleID int64,
	stage, op, promptStr string,
	settings genSettings,
) (string, articles.Usage, error) {
	done := s.stageTimer(
		ctx,
		articleID,
		stage,
		settings.provider,
		settings.model,
	)
	out, usage, err := settings.client.Complete(ctx, promptStr, settings.maxTokens)
	done(usage, err == nil)
	if err != nil {
		return "", usage, fmt.Errorf("%s: %w", op, err)
	}
	if strings.TrimSpace(out) == "" {
		return "", usage, fmt.Errorf("%s: empty response from %s/%s", op, settings.provider, settings.model)
	}
	return out, usage, nil
}

type checkOutcome struct {
	content        string
	last           *articles.CheckResult
	firstScore     *float64
	humanizeCycles int
	humanizeTokens int
}

func (c checkOutcome) passed() bool {
	return c.last != nil && c.last.Original
}

func (s *Service) checkAndHumanize(ctx context.Context, log *slog.Logger,
	articleID int64,
	settings genSettings,
	content string,
) checkOutcome {
	maxCycles := settings.maxCycles
	if maxCycles <= 0 {
		maxCycles = s.defaults.MaxCycles
	}
	threshold := settings.aiThreshold
	if threshold == 0 {
		threshold = s.defaults.AIThreshold
	}

	out := checkOutcome{content: content}

	for cycle := 1; cycle <= maxCycles; cycle++ {
		checkDone := s.stageTimer(
			ctx,
			articleID,
			articles.StageCheck,
			"checker",
			"",
		)
		checkRes, err := s.checker.Check(ctx, out.content)
		checkDone(articles.Usage{}, err == nil)
		if err != nil {
			log.DebugContext(ctx, "originality check failed, skipping further checks", "cycle", cycle, "err", err)
			break
		}

		passes := checkRes.AIScore < threshold
		checkRes.Original = passes
		out.last = checkRes
		if out.firstScore == nil {
			score := checkRes.AIScore
			out.firstScore = &score
		}

		log.DebugContext(ctx, "check result", "cycle", cycle, "ai_score", checkRes.AIScore, "threshold", threshold, "passes", passes)

		if saveErr := s.repo.SaveCheckResult(ctx, articleID, checkRes); saveErr != nil {
			log.DebugContext(ctx, "save check result failed, continuing", "err", saveErr)
		}

		if passes {
			return out
		}
		if cycle == maxCycles {
			log.InfoContext(ctx, "max humanize cycles reached, publishing as-is", "cycles", maxCycles, "final_ai_score", checkRes.AIScore)
			break
		}

		log.DebugContext(ctx, "content flagged — humanize rewrite", "cycle", cycle, "sentences_flagged", len(checkRes.SentencesFlagged))
		humanizeDone := s.stageTimer(
			ctx,
			articleID,
			articles.StageHumanize,
			settings.provider,
			settings.model,
		)
		rewritten, humanizeUsage, err := settings.client.Complete(ctx, prompt.Humanize(out.content, settings.keyword, checkRes.Issues, checkRes.SentencesFlagged), settings.maxTokens)
		humanizeDone(humanizeUsage, err == nil)
		out.humanizeTokens += humanizeUsage.Total()
		if err != nil {
			log.DebugContext(ctx, "humanize step failed, using current content", "cycle", cycle, "err", err)
			break
		}
		if strings.TrimSpace(rewritten) == "" {
			log.DebugContext(ctx, "humanize returned empty content, using current content", "cycle", cycle)
			break
		}
		out.content = rewritten
		out.humanizeCycles++
	}

	return out
}

func (s *Service) Publish(ctx context.Context, articleID int64) (articles.Article, error) {
	return s.publish(ctx, s.log, articleID)
}

func (s *Service) publish(ctx context.Context, log *slog.Logger, articleID int64) (articles.Article, error) {
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

	var params articleRequestParams
	if err := json.Unmarshal(article.RequestParams, &params); err != nil {
		return articles.Article{}, fmt.Errorf("decode stored request params: %w", err)
	}
	pub, err := s.publisher.ForSite(ctx, article.SiteID, params.Language)
	if err != nil {
		return articles.Article{}, fmt.Errorf("resolve publisher: %w", err)
	}

	publishDone := s.stageTimer(
		ctx,
		articleID,
		articles.StagePublish,
		"wordpress",
		"",
	)
	postURL, err := pub.Publish(ctx, article.WPPostID)
	publishDone(articles.Usage{}, err == nil)
	if err != nil {
		return articles.Article{}, fmt.Errorf("publish: %w", err)
	}

	if err := s.repo.MarkPublished(ctx, articleID, postURL); err != nil {
		if errors.Is(err, articles.ErrAlreadyPublished) {
			// Lost the race: another concurrent Publish() call for this same
			// article won between our status check above and this write. We
			// already called WordPress (unavoidable — the claim can't happen
			// before we know postURL), but at least don't silently overwrite
			// the winner's record with a second one.
			return articles.Article{}, ErrAlreadyPublished
		}
		return articles.Article{}, fmt.Errorf("mark published: %w", err)
	}

	updated, err := s.repo.Get(ctx, articleID)
	if err != nil {
		return articles.Article{}, fmt.Errorf("fetch article after publish: %w", err)
	}
	log.InfoContext(ctx, "article published", "article_id", articleID, "wp_post_id", updated.WPPostID, "wp_post_url", updated.WPPostURL)
	return *updated, nil
}

func (s *Service) List(ctx context.Context, limit, offset int) ([]articles.Article, int, error) {
	arts, total, err := s.repo.List(ctx, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list articles: %w", err)
	}
	return arts, total, nil
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

func estimateMaxTokens(maxWords int) int {
	return maxWords*3 + 200
}
