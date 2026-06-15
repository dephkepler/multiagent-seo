package app

import (
	"context"
	"log/slog"

	"github.com/rs/zerolog/log"

	appapitoken "multiagent-seo/internal/application/apitoken"
	apparticles "multiagent-seo/internal/application/articles"
	appauth "multiagent-seo/internal/application/auth"
	appwordpress "multiagent-seo/internal/application/wordpress"
	domainarticles "multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/domain/articles/prompt"
	domainauth "multiagent-seo/internal/domain/auth"
	"multiagent-seo/internal/infrastructure/checker"
	"multiagent-seo/internal/infrastructure/dataforseo"
	"multiagent-seo/internal/infrastructure/http/handlers"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/infrastructure/sheets"
	"multiagent-seo/pkg/config"
)

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

func seedWriterChampion(ctx context.Context, store *postgres.PromptRepository) {
	vars, err := store.ActiveVariants(ctx, domainarticles.PromptStageWriter)
	if err != nil {
		log.Warn().Err(err).Msg("check writer prompt champion")
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
		log.Warn().Err(err).Msg("seed writer prompt champion")
		return
	}
	log.Info().Msg("seeded writer prompt champion from built-in template")
}

func nilableWordpressService(svc *appwordpress.Service) handlers.WordpressService {
	if svc == nil {
		return nil
	}
	return svc
}

func nilableAuthService(svc *appauth.Service) handlers.AuthService {
	if svc == nil {
		return nil
	}
	return svc
}

func nilableArticlesHandler(svc *apparticles.Service) *handlers.ArticlesHandler {
	if svc == nil {
		return handlers.NewArticlesHandler(nil)
	}
	return handlers.NewArticlesHandler(svc)
}

func nilableApiTokensHandler(svc *appapitoken.Service) *handlers.ApiTokensHandler {
	if svc == nil {
		return handlers.NewApiTokensHandler(nil)
	}
	return handlers.NewApiTokensHandler(svc)
}

type compositeVerifier struct {
	jwt  domainauth.TokenVerifier
	keys *appapitoken.Service
}

func (c compositeVerifier) Verify(ctx context.Context, token string) (string, error) {
	if appapitoken.HasKeyPrefix(token) {
		uid, err := c.keys.Authenticate(ctx, token)
		if err != nil {
			return "", err
		}
		return uid.String(), nil
	}
	return c.jwt.Verify(ctx, token)
}
