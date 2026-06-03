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

	// JWT signing/verification needs no DB, so the issuer-verifier is always ready.
	jwtSvc := jwtauth.New(cfg.JWT.Secret, cfg.JWT.TTL)

	// A DB outage must not stop the server from booting — DB-backed features then
	// report 503/degraded per request instead of the process failing to start.
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

	// Link-building Flow 1/2 are Sheets-only; Flow 3 (place-backlinks) needs the
	// DB to cache donor app-passwords and the wordpress repo to resolve target
	// URLs. Both extra deps may be nil (no pool) — buildLinkbuilding handles it.
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

	// API keys authenticate alongside JWTs once the DB (their store) is up.
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

	// Stop accepting requests before draining background jobs: no new jobs can be
	// dispatched mid-drain, avoiding a WaitGroup Add-vs-Wait race in the runner.
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

// newSERP falls back to a mock when DataForSEO is unconfigured.
func newSERP(cfg config.Config, log *slog.Logger) domainarticles.SERPProvider {
	if cfg.DataForSEO.Login != "" && cfg.DataForSEO.Password != "" {
		return dataforseo.New(cfg.DataForSEO.Login, cfg.DataForSEO.Password)
	}
	log.Warn("dataforseo unconfigured; using SERP mock")
	return dataforseo.NewMock()
}

// newTopics falls back to a mock when Sheets is unconfigured or unreachable.
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

// newChecker falls back to the mock checker when the configured provider can't
// be built (e.g. missing key), matching the legacy degrade-to-mock behaviour.
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

// nilableLinkbuildingHandler hands the constructor the typed-nil pointers
// directly; the handler uses a reflect-based check that treats them as nil so
// the 503 path still fires for missing prerequisites.
func nilableLinkbuildingHandler(
	svc *applinkbuilding.Service,
	loginSvc *applinkbuilding.LoginService,
	backlinkSvc *applinkbuilding.BacklinkService,
) *handlers.LinkbuildingHandler {
	return handlers.NewLinkbuildingHandler(svc, loginSvc, backlinkSvc)
}

// buildLinkbuilding wires the link-building flows. Flow 1 (qualify) needs an
// LLM; Flow 2 (login) needs only the sheets credential source; Flow 3
// (place-backlinks) additionally needs the DB pool + wordpress repo to cache
// donor app-passwords and resolve target URLs. Any flow whose prerequisites
// are absent comes back nil so the endpoint reports 503 instead of crashing.
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

	// Qualification (Flow 1) — classifier built per request via the factory so
	// the run can target a different model than the deployment default.
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
		classifierBuilder,
		applinkbuilding.LLMDefaults{Provider: qualifyProvider, Model: qualifyModel},
		runner,
		log,
	)

	// Login (Flow 2) needs the credential source; it does not use the LLM.
	creds, err := sheets.NewCredentialSource(ctx, cfg.Sheets.CredentialsFile, cfg.Sheets.SpreadsheetID, log)
	if err != nil {
		log.Warn("link-building login disabled: credential source unavailable", "err", err)
		return qualifySvc, nil, nil, runner
	}
	loginSvc := applinkbuilding.NewLoginService(creds, wplogin.New(log), runner, log)

	// Place-backlinks (Flow 3) wants DB + wordpress repo for the target-URL
	// resolver, plus an LLM for the inline backlink rewrite.
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

// placerLLM is the same adapter pattern as classifierLLM — the backlink placer's
// port only needs the prose, not the token-usage triple.
type placerLLM struct{ c domainarticles.LLMClient }

func (a placerLLM) Complete(ctx context.Context, prompt string, maxTokens int) (string, error) {
	out, _, err := a.c.Complete(ctx, prompt, maxTokens)
	return out, err
}

// wordpressTargetResolver confirms the requested URL is one of our
// wordpress_sites (case-insensitive, trailing-slash insensitive) and returns
// the canonical stored form, so the backlink uses exactly what's in our DB.
type wordpressTargetResolver struct{ repo *postgres.WordpressSiteRepository }

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

// classifierLLM adapts the articles LLM client (whose Complete also returns token
// usage) to the topic classifier's minimal LLM interface.
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

// compositeVerifier authenticates a bearer value as either one of our API keys
// (recognised by prefix) or a JWT.
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
