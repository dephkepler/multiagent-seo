package root

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/rs/zerolog/log"

	appapitoken "multiagent-seo/internal/application/apitoken"
	appauth "multiagent-seo/internal/application/auth"
	appclientsegments "multiagent-seo/internal/application/clientsegments"
	apphealth "multiagent-seo/internal/application/health"
	appleadstats "multiagent-seo/internal/application/leadstats"
	appwordpress "multiagent-seo/internal/application/wordpress"
	domainauth "multiagent-seo/internal/domain/auth"
	domainhealth "multiagent-seo/internal/domain/health"
	domainleadstats "multiagent-seo/internal/domain/leadstats"
	"multiagent-seo/internal/infrastructure/db"
	"multiagent-seo/internal/infrastructure/ga4"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	httpMiddleware "multiagent-seo/internal/infrastructure/http/middleware"
	"multiagent-seo/internal/infrastructure/jwtauth"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/jobrunner"
	"multiagent-seo/pkg/logger"
	"multiagent-seo/pkg/sentry"
)

func Run(ctx context.Context, cfg config.Config) error {
	slogLog := logger.NewSlog()
	slog.SetDefault(slogLog)

	sentryClient := sentry.New(cfg.Sentry)
	if err := sentryClient.Initialize(); err != nil {
		return fmt.Errorf("sentry init: %w", err)
	}
	defer sentryClient.Flush()

	jwtSvc := jwtauth.New(cfg.JWT.Secret, cfg.JWT.TTL)

	database := db.NewDatabase(cfg.Database)
	if err := database.Initialize(ctx); err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer database.Close()
	pool := database.Pool()

	healthRepo := postgres.NewHealthRepository(pool)
	wordpressRepo := postgres.NewWordpressSiteRepository(pool, cfg.WordPress.EncryptionKey)
	wordpressSvc := appwordpress.NewService(wordpressRepo)
	authSvc := appauth.NewService(postgres.NewUserRepository(pool), jwtSvc)
	apiTokenSvc := appapitoken.NewService(postgres.NewApiTokenRepository(pool))

	articlesSvc, evolveSvc, articlesRunner := buildArticles(ctx, cfg, slogLog, pool, wordpressRepo)
	linkbuildingBacklinkSvc, lbRunner := buildLinkbuilding(ctx, cfg, slogLog, pool, wordpressRepo)
	emailScrapeSvc, emailRunner := buildEmailScrape(ctx, cfg, slogLog, pool)
	leadsSvc := buildLeads(ctx, cfg, slogLog, pool)
	adminBot := buildAdminBot(ctx, cfg, slogLog, pool, leadsSvc)
	var trafficSource domainleadstats.TrafficSource
	if cfg.GA4.PropertyID != "" {
		ga4Client, err := ga4.New(ctx, cfg.Sheets.CredentialsFile, cfg.GA4.PropertyID)
		if err != nil {
			slogLog.Warn("leadstats: ga4 disabled, client unavailable", "err", err)
		} else {
			trafficSource = ga4Client
		}
	}
	leadStatsSvc := appleadstats.NewService(postgres.NewLeadStatsRepository(pool), trafficSource, slogLog)
	clientSegmentsSvc := appclientsegments.NewService(postgres.NewClientSegmentsRepository(pool))

	healthSvc := apphealth.NewService(domainhealth.NewService(healthRepo))
	server := handlers.NewServer(
		handlers.NewHealthHandler(healthSvc),
		handlers.NewWordpressSitesHandler(wordpressSvc),
		handlers.NewLoginHandler(authSvc),
		handlers.NewArticlesHandler(articlesSvc),
		handlers.NewLinkbuildingHandler(linkbuildingBacklinkSvc),
		handlers.NewApiTokensHandler(apiTokenSvc),
		handlers.NewEmailScrapeHandler(emailScrapeSvc),
		handlers.NewLeadStatsHandler(leadStatsSvc),
		handlers.NewClientSegmentsHandler(clientSegmentsSvc),
	)

	schedule(ctx, cfg.Prompt.PromoteInterval, evolveSvc.PromotePrompts)
	if cfg.Prompt.EvolveEnabled {
		schedule(ctx, cfg.Prompt.EvolveInterval, evolveSvc.GenerateCandidate)
	}
	if leadsSvc != nil {
		schedule(ctx, cfg.Worker.PollInterval, leadsSvc.ProcessNewLeads)
	}
	if adminBot != nil {
		go adminBot.Run(ctx)
		schedule(ctx, cfg.Reminder.CheckInterval, adminBot.SendReminders)
	}

	var verifier domainauth.TokenVerifier = compositeVerifier{jwt: jwtSvc, keys: apiTokenSvc}
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
	for name, rn := range map[string]*jobrunner.AsyncRunner{"articles": articlesRunner, "linkbuilding": lbRunner, "emailscrape": emailRunner} {
		if rn == nil {
			continue
		}
		if err := rn.Wait(shutdownCtx); err != nil {
			log.Warn().Str("runner", name).Err(err).Msg("background jobs did not drain before timeout")
		}
	}
	return nil
}
