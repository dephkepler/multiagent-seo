package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"

	appauth "multiagent-seo/internal/application/auth"
	apphealth "multiagent-seo/internal/application/health"
	appwordpress "multiagent-seo/internal/application/wordpress"
	domainhealth "multiagent-seo/internal/domain/health"
	"multiagent-seo/internal/infrastructure/db"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	httpMiddleware "multiagent-seo/internal/infrastructure/http/middleware"
	"multiagent-seo/internal/infrastructure/jwtauth"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/logger"
	"multiagent-seo/pkg/sentry"
)

func main() {
	logger.Init()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config load")
	}

	sentryClient := sentry.New(cfg.Sentry)
	if err := sentryClient.Initialize(); err != nil {
		log.Fatal().Err(err).Msg("sentry init")
	}
	defer sentryClient.Flush()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// JWT signing/verification needs no DB, so the issuer-verifier is always ready.
	jwtSvc := jwtauth.New(cfg.JWT.Secret, cfg.JWT.TTL)

	// A DB outage must not stop the server from booting — /healthz then reports
	// degraded instead of the process failing to start.
	var healthRepo domainhealth.Repository
	var wordpressSvc *appwordpress.Service
	var authSvc *appauth.Service
	pool, err := db.NewPool(ctx, cfg.Database)
	if err != nil {
		log.Warn().Err(err).Msg("database unavailable at startup; health will report degraded")
	} else {
		defer pool.Close()
		healthRepo = postgres.NewHealthRepository(pool)
		wordpressRepo := postgres.NewWordpressSiteRepository(pool, cfg.WordPress.EncryptionKey)
		wordpressSvc = appwordpress.NewService(wordpressRepo)
		userRepo := postgres.NewUserRepository(pool)
		authSvc = appauth.NewService(userRepo, jwtSvc)
	}

	healthSvc := apphealth.NewService(domainhealth.NewService(healthRepo))
	// wordpressSvc/authSvc are nil without a DB; their handlers then return 503 per request.
	wordpressHandler := handlers.NewWordpressSitesHandler(nilableWordpressService(wordpressSvc))
	loginHandler := handlers.NewLoginHandler(nilableAuthService(authSvc))
	server := handlers.NewServer(handlers.NewHealthHandler(healthSvc), wordpressHandler, loginHandler)
	authMW := httpMiddleware.BearerAuth(jwtSvc)
	router := apihttp.NewRouter(cfg.Server, server, authMW)

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
}

// nilableWordpressService returns an untyped-nil interface when the service is
// absent, so the handler's nil guard fires instead of dereferencing a typed nil.
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
