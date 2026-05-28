package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"

	apphealth "contentflow/internal/application/health"
	appwordpress "contentflow/internal/application/wordpress"
	domainhealth "contentflow/internal/domain/health"
	"contentflow/internal/infrastructure/db"
	apihttp "contentflow/internal/infrastructure/http"
	"contentflow/internal/infrastructure/http/handlers"
	"contentflow/internal/infrastructure/persistence/postgres"
	"contentflow/pkg/config"
	"contentflow/pkg/logger"
	"contentflow/pkg/sentry"
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

	// A DB outage must not stop the server from booting — /healthz then reports
	// degraded instead of the process failing to start.
	var healthRepo domainhealth.Repository
	var wordpressSvc *appwordpress.Service
	pool, err := db.NewPool(ctx, cfg.Database)
	if err != nil {
		log.Warn().Err(err).Msg("database unavailable at startup; health will report degraded")
	} else {
		defer pool.Close()
		healthRepo = postgres.NewHealthRepository(pool)
		wordpressRepo := postgres.NewWordpressSiteRepository(pool, cfg.WordPress.EncryptionKey)
		wordpressSvc = appwordpress.NewService(wordpressRepo)
	}

	healthSvc := apphealth.NewService(domainhealth.NewService(healthRepo))
	// wordpressSvc is nil without a DB; its handler then returns 503 per request.
	wordpressHandler := handlers.NewWordpressSitesHandler(nilableWordpressService(wordpressSvc))
	server := handlers.NewServer(handlers.NewHealthHandler(healthSvc), wordpressHandler)
	router := apihttp.NewRouter(cfg.Server, server)

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
