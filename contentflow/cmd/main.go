package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"contentflow/internal/application"
	"contentflow/internal/config"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.NewConfig()
	if err != nil {
		log.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	app, err := application.New(cfg, log)
	if err != nil {
		log.Error("failed to init application", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Start(ctx); err != nil {
		log.Error("application error", "err", err)
		os.Exit(1)
	}
}
