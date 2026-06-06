package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"syscall"

	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	appapitoken "multiagent-seo/internal/application/apitoken"
	apparticles "multiagent-seo/internal/application/articles"
	appauth "multiagent-seo/internal/application/auth"
	apphealth "multiagent-seo/internal/application/health"
	applinkbuilding "multiagent-seo/internal/application/linkbuilding"
	appwordpress "multiagent-seo/internal/application/wordpress"
	domainarticles "multiagent-seo/internal/domain/articles"
	domainauth "multiagent-seo/internal/domain/auth"
	domainhealth "multiagent-seo/internal/domain/health"
	domainlinkbuilding "multiagent-seo/internal/domain/linkbuilding"
	"multiagent-seo/internal/infrastructure/backlinkplacer"
	"multiagent-seo/internal/infrastructure/checker"
	"multiagent-seo/internal/infrastructure/dataforseo"
	"multiagent-seo/internal/infrastructure/db"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	httpMiddleware "multiagent-seo/internal/infrastructure/http/middleware"
	"multiagent-seo/internal/infrastructure/jwtauth"
	infrallm "multiagent-seo/internal/infrastructure/llm"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/infrastructure/postsdiscover"
	"multiagent-seo/internal/infrastructure/pexels"
	"multiagent-seo/internal/infrastructure/sheets"
	"multiagent-seo/internal/infrastructure/topicclassifier"
	"multiagent-seo/internal/infrastructure/webfetch"
	infrawp "multiagent-seo/internal/infrastructure/wordpress"
	"multiagent-seo/internal/infrastructure/wplogin"
	"multiagent-seo/internal/infrastructure/wppost"
	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/jobrunner"
	"multiagent-seo/pkg/logger"
	"multiagent-seo/pkg/sentry"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config load")
	}

	if err := logger.Init(cfg.Logger.Level); err != nil {
		log.Fatal().Err(err).Msg("logger init")
	}
	slogLog := logger.NewSlog()
	slog.SetDefault(slogLog)

	sentryClient := sentry.New(cfg.Sentry)
	if err := sentryClient.Initialize(); err != nil {
		log.Fatal().Err(err).Msg("sentry init")
	}
	defer sentryClient.Flush()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	jwtSvc := jwtauth.New(cfg.JWT.Secret, cfg.JWT.TTL)

	var healthRepo domainhealth.Repository
	var wordpressSvc *appwordpress.Service
	var authSvc *appauth.Service
	var articlesSvc *apparticles.Service
	var apiTokenSvc *appapitoken.Service
	var runner *jobrunner.AsyncRunner
	var wordpressRepo *postgres.WordpressSiteRepository

	pool, err := db.NewPool(ctx, cfg.Database)
	if err != nil {
		log.Warn().Err(err).Msg("database unavailable at startup; DB-backed features will report 503")
	} else {
		defer pool.Close()
		healthRepo = postgres.NewHealthRepository(pool)
		wordpressRepo = postgres.NewWordpressSiteRepository(pool, cfg.WordPress.EncryptionKey)
		wordpressSvc = appwordpress.NewService(wordpressRepo)
		authSvc = appauth.NewService(postgres.NewUserRepository(pool), jwtSvc)
		apiTokenSvc = appapitoken.NewService(postgres.NewApiTokenRepository(pool))

		runner = jobrunner.NewAsyncRunner(cfg.Server.BackgroundJobTimeout, slogLog)
		articlesSvc = apparticles.NewService(
			postgres.NewArticleRepository(pool),
			infrallm.NewFactory(cfg.LLM, slogLog),
			newSERP(cfg, slogLog),
			newTopics(ctx, cfg, slogLog),
			newChecker(cfg, slogLog),
			pexels.New(cfg.Pexels.APIKey, slogLog),
			infrawp.NewProvider(wordpressRepo, slogLog),
			runner,
			articleDefaults(cfg),
			slogLog,
		)
	}

	linkbuildingSvc, linkbuildingLoginSvc, linkbuildingBacklinkSvc, lbRunner := buildLinkbuilding(ctx, cfg, slogLog, pool, wordpressRepo)

	healthSvc := apphealth.NewService(domainhealth.NewService(healthRepo))
	server := handlers.NewServer(
		handlers.NewHealthHandler(healthSvc),
		handlers.NewWordpressSitesHandler(nilableWordpressService(wordpressSvc)),
		handlers.NewLoginHandler(nilableAuthService(authSvc)),
		nilableArticlesHandler(articlesSvc),
		nilableLinkbuildingHandler(linkbuildingSvc, linkbuildingLoginSvc, linkbuildingBacklinkSvc),
		nilableApiTokensHandler(apiTokenSvc),
	)

	var verifier domainauth.TokenVerifier = jwtSvc
	if apiTokenSvc != nil {
		verifier = compositeVerifier{jwt: jwtSvc, keys: apiTokenSvc}
	}
	router := apihttp.NewRouter(cfg.Server, server, httpMiddleware.BearerAuth(verifier))

	srv := &http.Server{
		Addr:         net.JoinHostPort(cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		log.Info().Str("addr", srv.Addr).Msg("http server listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("http server")
		}
	}()

	<-ctx.Done()
	stop()
	log.Info().Msg("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownWaitTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("graceful shutdown failed")
	}
	for name, rn := range map[string]*jobrunner.AsyncRunner{"articles": runner, "linkbuilding": lbRunner} {
		if rn == nil {
			continue
		}
		if err := rn.Wait(shutdownCtx); err != nil {
			log.Warn().Str("runner", name).Err(err).Msg("background jobs did not drain before timeout")
		}
	}
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
	}
}

func newSERP(cfg config.Config, log *slog.Logger) domainarticles.SERPProvider {
	if cfg.DataForSEO.Login != "" && cfg.DataForSEO.Password != "" {
		return dataforseo.New(cfg.DataForSEO.Login, cfg.DataForSEO.Password)
	}
	log.Warn("dataforseo unconfigured; using SERP mock")
	return dataforseo.NewMock()
}

func newTopics(ctx context.Context, cfg config.Config, log *slog.Logger) domainarticles.TopicSource {
	if cfg.Sheets.SpreadsheetID == "" {
		log.Warn("sheets unconfigured; using topic mock")
		return sheets.NewMock()
	}
	ts, err := sheets.New(ctx, cfg.Sheets.CredentialsFile, cfg.Sheets.SpreadsheetID, cfg.Sheets.Sheet,
		cfg.Sheets.TopicColumn, cfg.Sheets.KeywordColumn, cfg.Sheets.TitleColumn, cfg.Sheets.HeaderRow, log)
	if err != nil {
		log.Warn("sheets init failed; using topic mock", "err", err)
		return sheets.NewMock()
	}
	return ts
}

func newChecker(cfg config.Config, log *slog.Logger) domainarticles.ContentChecker {
	c, err := checker.New(cfg.Checker.Provider, cfg.Checker.APIKey, cfg.Checker.Model, cfg.Checker.AIThreshold, log)
	if err != nil {
		log.Warn("checker init failed; using mock", "provider", cfg.Checker.Provider, "err", err)
		c, _ = checker.New("mock", "", "", cfg.Checker.AIThreshold, log)
	}
	return c
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
) (*applinkbuilding.Service, *applinkbuilding.LoginService, *applinkbuilding.BacklinkService, *jobrunner.AsyncRunner) {
	src, err := sheets.NewWebsiteSource(ctx, cfg.Sheets.CredentialsFile, cfg.Sheets.SpreadsheetID, log)
	if err != nil {
		log.Warn("link-building disabled: website source unavailable", "err", err)
		return nil, nil, nil, nil
	}
	runner := jobrunner.NewAsyncRunner(cfg.Server.BackgroundJobTimeout, log)
	factory := infrallm.NewFactory(cfg.LLM, log)

	classifierBuilder := func(provider, model string) (domainlinkbuilding.TopicClassifier, error) {
		client, err := factory.ForModel(provider, model)
		if err != nil {
			return nil, err
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
			return nil, err
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
		return "", err
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

type classifierLLM struct{ c domainarticles.LLMClient }

func (a classifierLLM) Complete(ctx context.Context, prompt string, maxTokens int) (string, error) {
	out, _, err := a.c.Complete(ctx, prompt, maxTokens)
	return out, err
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
