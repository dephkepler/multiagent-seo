package main

import (
	"context"
	"os/signal"
	"syscall"
	// Embeds the timezone database in the binary. The runtime image is bare
	// alpine with no tzdata, so time.LoadLocation("Europe/Kyiv") failed there —
	// which silently disabled the client portal and answered 503 to every
	// booking request while the logs showed one warning at startup. Embedding it
	// costs ~450 KB and cannot be lost by changing a base image.
	_ "time/tzdata"

	"github.com/rs/zerolog/log"

	"multiagent-seo/internal/root"
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

	if err := root.Run(ctx, cfg); err != nil {
		log.Fatal().Err(err).Msg("app run")
	}
}
