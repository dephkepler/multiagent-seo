package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"

	"multiagent-seo/internal/app"
	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config load")
	}

	if err := logger.Init(cfg.Logger.Level); err != nil {
		log.Fatal().Err(err).Msg("logger init")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, cfg); err != nil {
		log.Fatal().Err(err).Msg("app run")
	}
}
