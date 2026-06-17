package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	appapitoken "multiagent-seo/internal/application/apitoken"
	apparticles "multiagent-seo/internal/application/articles"
	appauth "multiagent-seo/internal/application/auth"
	apphealth "multiagent-seo/internal/application/health"
	appwordpress "multiagent-seo/internal/application/wordpress"
	domainauth "multiagent-seo/internal/domain/auth"
	domainhealth "multiagent-seo/internal/domain/health"
	"multiagent-seo/internal/infrastructure/db"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	httpMiddleware "multiagent-seo/internal/infrastructure/http/middleware"
	"multiagent-seo/internal/infrastructure/jwtauth"
	infrallm "multiagent-seo/internal/infrastructure/llm"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/infrastructure/pexels"
	infrawp "multiagent-seo/internal/infrastructure/wordpress"
	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/jobrunner"
	"multiagent-seo/pkg/logger"
	"multiagent-seo/pkg/sentry"
)

const promoteInterval = 6 * time.Hour

const evolveInterval = 7 * 24 * time.Hour

func Run(ctx context.Context, cfg config.Config) error {
	slogLog := logger.NewSlog()
	slog.SetDefault(slogLog)

	sentryClient := sentry.New(cfg.Sentry)
	if err := sentryClient.Initialize(); err != nil {
		return fmt.Errorf("sentry init: %w", err)
	}
	defer sentryClient.Flush()

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
		log.Warn().Err(err).
			Str("db_host", cfg.Database.Host).
			Str("db_port", cfg.Database.Port).
			Str("db_name", cfg.Database.Dbname).
			Msg("database unavailable at startup; DB-backed features will report 503")
	} else {
		defer pool.Close()
		healthRepo = postgres.NewHealthRepository(pool)
		wordpressRepo = postgres.NewWordpressSiteRepository(pool, cfg.WordPress.EncryptionKey)
		wordpressSvc = appwordpress.NewService(wordpressRepo)
		authSvc = appauth.NewService(postgres.NewUserRepository(pool), jwtSvc)
		apiTokenSvc = appapitoken.NewService(postgres.NewApiTokenRepository(pool))

		runner = jobrunner.NewAsyncRunner(cfg.Server.BackgroundJobTimeout, cfg.Server.BackgroundJobConcurrency, slogLog)
		articleRepo := postgres.NewArticleRepository(pool)
		promptRepo := postgres.NewPromptRepository(pool)
		articlesSvc = apparticles.NewService(
			articleRepo,
			infrallm.NewFactory(cfg.LLM, slogLog),
			newSERP(cfg, slogLog),
			newTopics(ctx, cfg, slogLog),
			newChecker(cfg, slogLog),
			pexels.New(cfg.Pexels.APIKey, slogLog),
			infrawp.NewProvider(wordpressRepo, slogLog),
			promptRepo,
			runner,
			articleDefaults(cfg),
			slogLog,
		)

		if n, err := articleRepo.FailOrphanedGenerating(ctx); err != nil {
			log.Warn().Err(err).Msg("reconcile orphaned generating articles")
		} else if n > 0 {
			log.Info().Int64("count", n).Msg("reconciled orphaned generating articles to failed")
		}

		seedWriterChampion(ctx, promptRepo)
	}

	linkbuildingSvc, linkbuildingLoginSvc, linkbuildingBacklinkSvc, lbRunner := buildLinkbuilding(ctx, cfg, slogLog, pool, wordpressRepo)
	emailScrapeSvc, emailRunner := buildEmailScrape(ctx, cfg, slogLog, pool)

	healthSvc := apphealth.NewService(domainhealth.NewService(healthRepo))
	server := handlers.NewServer(
		handlers.NewHealthHandler(healthSvc),
		handlers.NewWordpressSitesHandler(nilableWordpressService(wordpressSvc)),
		handlers.NewLoginHandler(nilableAuthService(authSvc)),
		nilableArticlesHandler(articlesSvc),
		nilableLinkbuildingHandler(linkbuildingSvc, linkbuildingLoginSvc, linkbuildingBacklinkSvc),
		nilableApiTokensHandler(apiTokenSvc),
		nilableEmailScrapeHandler(emailScrapeSvc),
	)

	if articlesSvc != nil {
		schedule(ctx, promoteInterval, articlesSvc.PromotePrompts)
		if cfg.Prompt.EvolveEnabled {
			schedule(ctx, evolveInterval, articlesSvc.GenerateCandidate)
		}
	}

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
		log.Info().
			Str("addr", srv.Addr).
			Msg("http server listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().
				Err(err).
				Msg("http server")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownWaitTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("graceful shutdown failed")
	}
	for name, rn := range map[string]*jobrunner.AsyncRunner{"articles": runner, "linkbuilding": lbRunner, "emailscrape": emailRunner} {
		if rn == nil {
			continue
		}
		if err := rn.Wait(shutdownCtx); err != nil {
			log.Warn().Str("runner", name).Err(err).Msg("background jobs did not drain before timeout")
		}
	}
	return nil
}
