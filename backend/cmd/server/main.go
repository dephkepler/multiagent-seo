package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"syscall"

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

	pool, err := db.NewPool(ctx, cfg.Database)
	if err != nil {
		log.Warn().Err(err).Msg("database unavailable at startup; DB-backed features will report 503")
	} else {
		defer pool.Close()
		healthRepo = postgres.NewHealthRepository(pool)
		wordpressRepo := postgres.NewWordpressSiteRepository(pool, cfg.WordPress.EncryptionKey)
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

	// Link-building is Sheets-only (no DB), so it's built independently of the pool.
	linkbuildingSvc, linkbuildingLoginSvc, lbRunner := buildLinkbuilding(ctx, cfg, slogLog)

	healthSvc := apphealth.NewService(domainhealth.NewService(healthRepo))
	server := handlers.NewServer(
		handlers.NewHealthHandler(healthSvc),
		handlers.NewWordpressSitesHandler(nilableWordpressService(wordpressSvc)),
		handlers.NewLoginHandler(nilableAuthService(authSvc)),
		nilableArticlesHandler(articlesSvc),
		nilableLinkbuildingHandler(linkbuildingSvc, linkbuildingLoginSvc),
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

// nilableLinkbuildingHandler passes untyped nil for whichever service is
// absent, so the handler's nil checks (which return 503) work — a typed-nil
// pointer stored in an interface would not compare equal to nil.
func nilableLinkbuildingHandler(svc *applinkbuilding.Service, loginSvc *applinkbuilding.LoginService) *handlers.LinkbuildingHandler {
	switch {
	case svc != nil && loginSvc != nil:
		return handlers.NewLinkbuildingHandler(svc, loginSvc)
	case svc != nil:
		return handlers.NewLinkbuildingHandler(svc, nil)
	case loginSvc != nil:
		return handlers.NewLinkbuildingHandler(nil, loginSvc)
	default:
		return handlers.NewLinkbuildingHandler(nil, nil)
	}
}

// buildLinkbuilding wires the Sheets-only link-building feature. It returns nil
// when its prerequisites (a writable Sheets website-source, an LLM for the topic
// classifier) are absent, so the endpoint reports 503 instead of failing boot.
func buildLinkbuilding(ctx context.Context, cfg config.Config, log *slog.Logger) (*applinkbuilding.Service, *applinkbuilding.LoginService, *jobrunner.AsyncRunner) {
	src, err := sheets.NewWebsiteSource(ctx, cfg.Sheets.CredentialsFile, cfg.Sheets.SpreadsheetID, log)
	if err != nil {
		log.Warn("link-building disabled: website source unavailable", "err", err)
		return nil, nil, nil
	}
	runner := jobrunner.NewAsyncRunner(cfg.Server.BackgroundJobTimeout, log)

	// Qualification (Flow 1) needs the LLM topic classifier.
	var qualifySvc *applinkbuilding.Service
	if client, err := infrallm.NewFactory(cfg.LLM, log).ForModel(cfg.LLM.Provider, cfg.LLM.Model); err != nil {
		log.Warn("link-building qualify disabled: llm classifier unavailable", "err", err)
	} else {
		qualifySvc = applinkbuilding.NewService(
			src,
			webfetch.New(log),
			topicclassifier.New(classifierLLM{client}, log),
			runner,
			log,
		)
	}

	// Login (Flow 2) needs the credential source; it does not use the LLM.
	var loginSvc *applinkbuilding.LoginService
	if creds, err := sheets.NewCredentialSource(ctx, cfg.Sheets.CredentialsFile, cfg.Sheets.SpreadsheetID, log); err != nil {
		log.Warn("link-building login disabled: credential source unavailable", "err", err)
	} else {
		loginSvc = applinkbuilding.NewLoginService(creds, wplogin.New(log), runner, log)
	}

	return qualifySvc, loginSvc, runner
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
