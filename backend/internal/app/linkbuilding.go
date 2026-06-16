package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	applinkbuilding "multiagent-seo/internal/application/linkbuilding"
	domainarticles "multiagent-seo/internal/domain/articles"
	domainlinkbuilding "multiagent-seo/internal/domain/linkbuilding"
	"multiagent-seo/internal/infrastructure/backlinkplacer"
	"multiagent-seo/internal/infrastructure/http/handlers"
	infrallm "multiagent-seo/internal/infrastructure/llm"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/infrastructure/postsdiscover"
	"multiagent-seo/internal/infrastructure/sheets"
	"multiagent-seo/internal/infrastructure/topicclassifier"
	"multiagent-seo/internal/infrastructure/webfetch"
	"multiagent-seo/internal/infrastructure/wplogin"
	"multiagent-seo/internal/infrastructure/wppost"
	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/jobrunner"
)

func nilableLinkbuildingHandler(
	svc *applinkbuilding.Service,
	loginSvc *applinkbuilding.LoginService,
	backlinkSvc *applinkbuilding.BacklinkService,
) *handlers.LinkbuildingHandler {
	return handlers.NewLinkbuildingHandler(svc, loginSvc, backlinkSvc)
}

func buildLinkbuilding(
	ctx context.Context,
	cfg config.Config,
	log *slog.Logger,
	pool *pgxpool.Pool,
	wordpressRepo *postgres.WordpressSiteRepository,
) (*applinkbuilding.Service,
	*applinkbuilding.LoginService,
	*applinkbuilding.BacklinkService,
	*jobrunner.AsyncRunner,
) {
	src, err := sheets.NewWebsiteSource(ctx, cfg.Sheets.CredentialsFile, cfg.Sheets.SpreadsheetID, log)
	if err != nil {
		log.Warn("link-building disabled: website source unavailable", "err", err)
		return nil, nil, nil, nil
	}
	runner := jobrunner.NewAsyncRunner(cfg.Server.BackgroundJobTimeout, cfg.Server.BackgroundJobConcurrency, log)
	factory := infrallm.NewFactory(cfg.LLM, log)

	classifierBuilder := func(provider, model string) (domainlinkbuilding.TopicClassifier, error) {
		client, err := factory.ForModel(provider, model)
		if err != nil {
			return nil, fmt.Errorf("build topic classifier (provider=%q, model=%q): %w", provider, model, err)
		}
		return topicclassifier.New(classifierLLM{client}, log), nil
	}
	qualifyProvider, qualifyModel := cfg.LLM.DefaultsFor(config.TaskQualify)
	qualifySvc := applinkbuilding.NewService(
		src,
		webfetch.New(log),
		postsdiscover.New(log),
		classifierBuilder,
		applinkbuilding.LLMDefaults{Provider: qualifyProvider, Model: qualifyModel},
		runner,
		log,
	)

	creds, err := sheets.NewCredentialSource(ctx, cfg.Sheets.CredentialsFile, cfg.Sheets.SpreadsheetID, log)
	if err != nil {
		log.Warn("link-building login disabled: credential source unavailable", "err", err)
		return qualifySvc, nil, nil, runner
	}
	loginSvc := applinkbuilding.NewLoginService(creds, wplogin.New(log), runner, log)

	if pool == nil || wordpressRepo == nil {
		return qualifySvc, loginSvc, nil, runner
	}
	placements, err := sheets.NewPlacementSink(ctx, cfg.Sheets.CredentialsFile, cfg.Sheets.SpreadsheetID, log)
	if err != nil {
		log.Warn("link-building backlinks disabled: placement sink unavailable", "err", err)
		return qualifySvc, loginSvc, nil, runner
	}
	placerBuilder := func(provider, model string) (domainlinkbuilding.BacklinkPlacer, error) {
		client, err := factory.ForModel(provider, model)
		if err != nil {
			return nil, fmt.Errorf("build backlink placer (provider=%q, model=%q): %w", provider, model, err)
		}
		return backlinkplacer.New(placerLLM{client}, log), nil
	}
	backlinkProvider, backlinkModel := cfg.LLM.DefaultsFor(config.TaskBacklink)
	backlinkSvc := applinkbuilding.NewBacklinkService(
		creds,
		placements,
		postgres.NewDonorCredentialRepository(pool, cfg.WordPress.EncryptionKey),
		wplogin.New(log),
		wppost.New(log),
		placerBuilder,
		applinkbuilding.LLMDefaults{Provider: backlinkProvider, Model: backlinkModel},
		wordpressTargetResolver{repo: wordpressRepo},
		runner,
		log,
	)
	return qualifySvc, loginSvc, backlinkSvc, runner
}

type placerLLM struct{ c domainarticles.LLMClient }

func (a placerLLM) Complete(ctx context.Context, prompt string, maxTokens int) (string, error) {
	out, _, err := a.c.Complete(ctx, prompt, maxTokens)
	return out, err
}

type classifierLLM struct{ c domainarticles.LLMClient }

func (a classifierLLM) Complete(ctx context.Context, prompt string, maxTokens int) (string, error) {
	out, _, err := a.c.Complete(ctx, prompt, maxTokens)
	return out, err
}

type wordpressTargetResolver struct {
	repo *postgres.WordpressSiteRepository
}

func (r wordpressTargetResolver) ResolveSiteURL(ctx context.Context, raw string) (string, error) {
	want := normalizeSiteURL(raw)
	if want == "" {
		return "", fmt.Errorf("invalid target site url")
	}
	sites, err := r.repo.List(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve target site url: list wordpress sites: %w", err)
	}
	for _, s := range sites {
		if normalizeSiteURL(s.URL) == want {
			return s.URL, nil
		}
	}
	return "", fmt.Errorf("target_site_url %q is not in wordpress_sites", raw)
}

func normalizeSiteURL(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimSuffix(s, "/")
	return s
}
