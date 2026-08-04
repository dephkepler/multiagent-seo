package root

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	apparticles "multiagent-seo/internal/application/articles"
	"multiagent-seo/internal/application/promptevolution"
	domainarticles "multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/domain/articles/prompt"
	"multiagent-seo/internal/infrastructure/checker"
	"multiagent-seo/internal/infrastructure/dataforseo"
	infrallm "multiagent-seo/internal/infrastructure/llm"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/infrastructure/pexels"
	"multiagent-seo/internal/infrastructure/sheets"
	infrawp "multiagent-seo/internal/infrastructure/wordpress"
	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/jobrunner"
)

func buildArticles(ctx context.Context, cfg config.Config, log *slog.Logger, pool *pgxpool.Pool, wordpressRepo *postgres.WordpressSiteRepository) (*apparticles.Service, *promptevolution.Service, *jobrunner.AsyncRunner) {
	runner := jobrunner.NewAsyncRunner(cfg.Server.BackgroundJobTimeout, cfg.Server.BackgroundJobConcurrency, log)
	articleRepo := postgres.NewArticleRepository(pool)
	promptRepo := postgres.NewPromptRepository(pool)
	llmFactory := infrallm.NewFactory(cfg.LLM, log)

	svc := apparticles.NewService(
		articleRepo,
		llmFactory,
		newSERP(cfg, log),
		newTopics(ctx, cfg, log),
		newChecker(cfg, log),
		pexels.New(cfg.Pexels.APIKey, log),
		infrawp.NewProvider(wordpressRepo, log, cfg.WordPress.HTTPTimeout),
		promptRepo,
		runner,
		articleDefaults(cfg),
		log,
	)

	// Articles left "generating" by a previous crash are reconciled to failed on boot.
	if n, err := articleRepo.FailOrphanedGenerating(ctx); err != nil {
		log.Warn("reconcile orphaned generating articles", "err", err)
	} else if n > 0 {
		log.Info("reconciled orphaned generating articles to failed", "count", n)
	}

	seedWriterChampion(ctx, promptRepo, log)
	evolveSvc := promptevolution.NewService(promptRepo, llmFactory, log)
	return svc, evolveSvc, runner
}

func articleDefaults(cfg config.Config) apparticles.Defaults {
	return apparticles.Defaults{
		MinWords:      cfg.Article.MinWords,
		MaxWords:      cfg.Article.MaxWords,
		Language:      cfg.Article.Language,
		Provider:      cfg.LLM.Provider,
		Model:         cfg.LLM.Model,
		AIThreshold:   cfg.Checker.AIThreshold,
		MaxCycles:     cfg.Checker.MaxCycles,
		SERPLimit:     cfg.DataForSEO.SERPLimit,
		SiteTopic:     cfg.Article.SiteTopic,
		ExtraRules:    cfg.Article.ExtraRules,
		IncludeImages: cfg.Pexels.Enabled,
		HumanWeight:   cfg.Prompt.HumanWeight,
	}
}

func newSERP(cfg config.Config, log *slog.Logger) domainarticles.SERPProvider {
	if cfg.DataForSEO.Login != "" && cfg.DataForSEO.Password != "" {
		return dataforseo.New(cfg.DataForSEO.Login, cfg.DataForSEO.Password)
	}
	log.Info("dataforseo unconfigured, using SERP mock")
	return dataforseo.NewMock()
}

func newTopics(ctx context.Context, cfg config.Config, log *slog.Logger) domainarticles.TopicSource {
	if cfg.Sheets.SpreadsheetID == "" {
		log.Info("sheets unconfigured, using topic mock")
		return sheets.NewMock()
	}
	ts, err := sheets.New(ctx, cfg.Sheets.CredentialsFile, cfg.Sheets.SpreadsheetID, cfg.Sheets.Sheet,
		cfg.Sheets.TopicColumn, cfg.Sheets.KeywordColumn, cfg.Sheets.TitleColumn, cfg.Sheets.HeaderRow, log)
	if err != nil {
		log.Info("sheets init failed, using topic mock", "err", err)
		return sheets.NewMock()
	}
	return ts
}

func newChecker(cfg config.Config, log *slog.Logger) domainarticles.ContentChecker {
	c, err := checker.New(checker.Config{
		Provider:    cfg.Checker.Provider,
		Email:       cfg.Checker.Copyleaks.Email,
		APIKey:      cfg.Checker.APIKey,
		Model:       cfg.Checker.Model,
		AIThreshold: cfg.Checker.AIThreshold,
		Sandbox:     cfg.Checker.Copyleaks.Sandbox,
	}, log)
	if err != nil {
		log.Info("checker init failed, using mock", "provider", cfg.Checker.Provider, "err", err)
		c, err = checker.New(checker.Config{Provider: "mock", AIThreshold: cfg.Checker.AIThreshold}, log)
		if err != nil {
			log.Error("mock checker init failed", "err", err)
		}
	}
	return c
}

func seedWriterChampion(ctx context.Context, store *postgres.PromptRepository, log *slog.Logger) {
	vars, err := store.ActiveVariants(ctx, domainarticles.PromptStageWriter)
	if err != nil {
		log.Warn("check writer prompt champion", "err", err)
		return
	}
	for _, v := range vars {
		if v.Status == domainarticles.VariantChampion {
			return
		}
	}
	if _, err := store.InsertVariant(ctx, domainarticles.PromptVariant{
		Stage:  domainarticles.PromptStageWriter,
		Body:   prompt.WriterTemplate,
		Status: domainarticles.VariantChampion,
		Origin: domainarticles.OriginSeed,
	}); err != nil {
		log.Warn("seed writer prompt champion", "err", err)
		return
	}
	log.Info("seeded writer prompt champion from built-in template")
}
