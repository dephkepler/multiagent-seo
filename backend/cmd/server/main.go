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

	apparticles "multiagent-seo/internal/application/articles"
	appauth "multiagent-seo/internal/application/auth"
	apphealth "multiagent-seo/internal/application/health"
	appwordpress "multiagent-seo/internal/application/wordpress"
	domainarticles "multiagent-seo/internal/domain/articles"
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
	infrawp "multiagent-seo/internal/infrastructure/wordpress"
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

	healthSvc := apphealth.NewService(domainhealth.NewService(healthRepo))
	server := handlers.NewServer(
		handlers.NewHealthHandler(healthSvc),
		handlers.NewWordpressSitesHandler(nilableWordpressService(wordpressSvc)),
		handlers.NewLoginHandler(nilableAuthService(authSvc)),
		nilableArticlesHandler(articlesSvc),
	)
	router := apihttp.NewRouter(cfg.Server, server, httpMiddleware.BearerAuth(jwtSvc))

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
	if runner != nil {
		if err := runner.Wait(shutdownCtx); err != nil {
			log.Warn().Err(err).Msg("background jobs did not drain before timeout")
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
