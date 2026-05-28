package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"multiagent-seo/internal/application"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	app := application.New(log)

	if err := app.Start(ctx); err != nil {
		log.Error("failed to start", "err", err)
		os.Exit(1)
	}

	if err := app.Wait(ctx, cancel); err != nil {
		log.Error("application error", "err", err)
		os.Exit(1)
	}
}
