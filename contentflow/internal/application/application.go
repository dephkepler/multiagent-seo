package application

import (
	"context"
	"fmt"
	"log/slog"

	"contentflow/internal/config"
)

type Application struct {
	cfg *config.Config
	log *slog.Logger
}

func New(cfg *config.Config, log *slog.Logger) (*Application, error) {
	app := &Application{
		cfg: cfg,
		log: log,
	}

	if err := app.init(); err != nil {
		return nil, fmt.Errorf("init application: %w", err)
	}

	return app, nil
}

func (a *Application) init() error {
	// TODO: init repo
	// TODO: init sheets client
	// TODO: init llm client
	// TODO: init wordpress client
	// TODO: init service
	// TODO: init bot
	return nil
}

func (a *Application) Start(ctx context.Context) error {
	a.log.Info("contentflow started")

	// TODO: start bot
	// TODO: start pipeline worker

	<-ctx.Done()

	a.log.Info("contentflow shutting down")
	return nil
}
