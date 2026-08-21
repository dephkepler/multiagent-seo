package root

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/rs/zerolog/log"

	appadvocateview "multiagent-seo/internal/application/advocateview"
	appapitoken "multiagent-seo/internal/application/apitoken"
	appauth "multiagent-seo/internal/application/auth"
	appclientdetail "multiagent-seo/internal/application/clientdetail"
	appclientsegments "multiagent-seo/internal/application/clientsegments"
	appfinance "multiagent-seo/internal/application/finance"
	apphealth "multiagent-seo/internal/application/health"
	appleadstats "multiagent-seo/internal/application/leadstats"
	appvault "multiagent-seo/internal/application/vault"
	appwordpress "multiagent-seo/internal/application/wordpress"
	domainauth "multiagent-seo/internal/domain/auth"
	domainhealth "multiagent-seo/internal/domain/health"
	domainleadstats "multiagent-seo/internal/domain/leadstats"
	domainuser "multiagent-seo/internal/domain/user"
	"multiagent-seo/internal/infrastructure/db"
	"multiagent-seo/internal/infrastructure/ga4"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	httpMiddleware "multiagent-seo/internal/infrastructure/http/middleware"
	"multiagent-seo/internal/infrastructure/jwtauth"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/infrastructure/tgauth"
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
	userRepo := postgres.NewUserRepository(pool)
	authSvc := appauth.NewService(userRepo, jwtSvc)
	apiTokenSvc := appapitoken.NewService(postgres.NewApiTokenRepository(pool))

	articlesSvc, evolveSvc, articlesRunner := buildArticles(ctx, cfg, slogLog, pool, wordpressRepo)
	linkbuildingBacklinkSvc, lbRunner := buildLinkbuilding(ctx, cfg, slogLog, pool, wordpressRepo)
	emailScrapeSvc, emailRunner := buildEmailScrape(ctx, cfg, slogLog, pool)
	leadsSvc := buildLeads(ctx, cfg, slogLog, pool)
	clientSegmentsSvc := appclientsegments.NewService(postgres.NewClientSegmentsRepository(pool))
	adminBot := buildAdminBot(ctx, cfg, slogLog, pool, leadsSvc, clientSegmentsSvc)
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
	clientDetailSvc := appclientdetail.NewService(
		postgres.NewClientDetailRepository(pool, cfg.Clients.EncryptionKey),
		postgres.NewConsultationRepository(pool, cfg.Clients.EncryptionKey),
	)
	vaultSvc := appvault.NewService(postgres.NewVaultRepository(pool))
	advocateViewSvc := appadvocateview.NewService(postgres.NewAdvocateViewRepository(pool))

	financeRepo := postgres.NewFinanceRepository(pool)
	financeSvc := appfinance.NewService(appfinance.Deps{
		Categories: financeRepo,
		Expenses:   financeRepo,
		Rules:      financeRepo,
		Income:     financeRepo,
		Report:     financeRepo,
		Advocates:  postgres.NewAdvocateRateRepository(pool),
		Log:        slogLog,
	})

	healthSvc := apphealth.NewService(domainhealth.NewService(healthRepo))
	server := handlers.NewServer(handlers.Deps{
		Health:         handlers.NewHealthHandler(healthSvc),
		Wordpress:      handlers.NewWordpressSitesHandler(wordpressSvc),
		Login:          handlers.NewLoginHandler(authSvc),
		Articles:       handlers.NewArticlesHandler(articlesSvc),
		Linkbuilding:   handlers.NewLinkbuildingHandler(linkbuildingBacklinkSvc),
		ApiTokens:      handlers.NewApiTokensHandler(apiTokenSvc),
		EmailScrape:    handlers.NewEmailScrapeHandler(emailScrapeSvc),
		LeadStats:      handlers.NewLeadStatsHandler(leadStatsSvc),
		ClientSegments: handlers.NewClientSegmentsHandler(clientSegmentsSvc),
		ClientDetail:   handlers.NewClientDetailHandler(clientDetailSvc),
		Vault:          handlers.NewVaultHandler(vaultSvc),
		Finance:        handlers.NewFinanceHandler(financeSvc),
		My:             handlers.NewMyHandler(advocateViewSvc),
	})

	schedule(ctx, cfg.Finance.GenerateInterval, financeSvc.GenerateDueExpenses)
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

	// Without a bot token there is nothing to check a launch signature
	// against, so the Telegram scheme stays unavailable rather than taking the
	// whole server down — same treatment the bot itself gets above.
	var launches httpMiddleware.LaunchVerifier
	var tgSubjects domainuser.TelegramRepository
	if cfg.Telegram.BotToken == "" {
		log.Warn().Msg("telegram bot token not set, mini app authentication disabled")
	} else {
		launchVerifier, err := tgauth.NewVerifier(cfg.Telegram.BotToken, cfg.Telegram.InitDataMaxAge)
		if err != nil {
			return fmt.Errorf("telegram launch verifier: %w", err)
		}
		launches = launchVerifier
		tgSubjects = postgres.NewTelegramRepository(pool)
	}

	router := apihttp.NewRouter(
		cfg.Server,
		server,
		httpMiddleware.Authenticate(verifier, userRepo, launches, tgSubjects),
	)

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
