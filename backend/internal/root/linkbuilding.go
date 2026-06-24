package root

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
	infrallm "multiagent-seo/internal/infrastructure/llm"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/infrastructure/sheets"
	"multiagent-seo/internal/infrastructure/wplogin"
	"multiagent-seo/internal/infrastructure/wppost"
	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/jobrunner"
)

func buildLinkbuilding(
	ctx context.Context,
	cfg config.Config,
	log *slog.Logger,
	pool *pgxpool.Pool,
	wordpressRepo *postgres.WordpressSiteRepository,
) (*applinkbuilding.LoginService,
	*applinkbuilding.BacklinkService,
	*jobrunner.AsyncRunner,
) {
	creds, err := sheets.NewCredentialSource(ctx, cfg.Sheets.CredentialsFile, cfg.Sheets.SpreadsheetID, log)
	if err != nil {
		log.Warn("link-building disabled: credential source unavailable", "err", err)
		return nil, nil, nil
	}
	runner := jobrunner.NewAsyncRunner(cfg.Server.BackgroundJobTimeout, cfg.Server.BackgroundJobConcurrency, log)
	factory := infrallm.NewFactory(cfg.LLM, log)
	loginSvc := applinkbuilding.NewLoginService(creds, wplogin.New(log, cfg.WordPress.HTTPTimeout), runner, log,
		applinkbuilding.WithLoginDelay(cfg.LinkBuilding.LoginDelayMin, cfg.LinkBuilding.LoginDelayMax))

	placements, err := sheets.NewPlacementSink(ctx, cfg.Sheets.CredentialsFile, cfg.Sheets.SpreadsheetID, log)
	if err != nil {
		log.Warn("link-building backlinks disabled: placement sink unavailable", "err", err)
		return loginSvc, nil, runner
	}
	placerBuilder := func(provider, model string) (domainlinkbuilding.BacklinkPlacer, error) {
		client, err := factory.ForModel(provider, model)
		if err != nil {
			return nil, fmt.Errorf("build backlink placer (provider=%q, model=%q): %w", provider, model, err)
		}
		return backlinkplacer.New(placerLLM{client}, log), nil
	}
	backlinkProvider, backlinkModel := infrallm.DefaultsFor(cfg.LLM, infrallm.TaskBacklink)
	backlinkSvc := applinkbuilding.NewBacklinkService(
		creds,
		placements,
		postgres.NewBacklinkPlacementRepository(pool),
		postgres.NewDonorSiteProfileRepository(pool),
		postgres.NewDonorCredentialRepository(pool, cfg.WordPress.EncryptionKey),
		wplogin.New(log, cfg.WordPress.HTTPTimeout),
		wppost.New(log, cfg.WordPress.HTTPTimeout),
		placerBuilder,
		applinkbuilding.LLMDefaults{Provider: backlinkProvider, Model: backlinkModel},
		wordpressTargetResolver{repo: wordpressRepo},
		runner,
		log,
		applinkbuilding.WithBacklinkDelay(cfg.LinkBuilding.PlaceDelayMin, cfg.LinkBuilding.PlaceDelayMax),
		applinkbuilding.WithCooldown(cfg.LinkBuilding.LockedCooldown, cfg.LinkBuilding.FailCooldown),
		applinkbuilding.WithTierDelay(cfg.LinkBuilding.TierDelay),
	)
	return loginSvc, backlinkSvc, runner
}

type placerLLM struct{ c domainarticles.LLMClient }

func (a placerLLM) Complete(ctx context.Context, prompt string, maxTokens int) (string, error) {
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
