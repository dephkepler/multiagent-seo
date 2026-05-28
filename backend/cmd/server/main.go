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
	pool, err := db.NewPool(ctx, cfg.Database)
	if err != nil {
		log.Warn().Err(err).Msg("database unavailable at startup; health will report degraded")
	} else {
		defer pool.Close()
		healthRepo = postgres.NewHealthRepository(pool)
	}

	healthSvc := apphealth.NewService(domainhealth.NewService(healthRepo))
	server := handlers.NewServer(handlers.NewHealthHandler(healthSvc))
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
