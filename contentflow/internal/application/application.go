package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"contentflow/internal/checker"
	"contentflow/internal/checker/huggingface"
	"contentflow/internal/config"
	"contentflow/internal/dataforseo"
	"contentflow/internal/llm"
	"contentflow/internal/repo"
	"contentflow/internal/server"
	"contentflow/internal/sheets"
	"contentflow/internal/wordpress"
)

type Application struct {
	cfg     *config.Config
	log     *slog.Logger
	repo    *repo.Repo
	llm     llm.Client
	wp      wordpress.Client
	sh      sheets.Client
	serp    dataforseo.Client
	checker checker.Client
	srv     *server.Server
	wg      sync.WaitGroup
}

func New(log *slog.Logger) *Application {
	return &Application{log: log}
}

func (a *Application) Start(ctx context.Context) error {
	if err := a.initConfig(); err != nil {
		return fmt.Errorf("init config: %w", err)
	}
	if err := a.initRepo(ctx); err != nil {
		return fmt.Errorf("init repo: %w", err)
	}
	if err := a.initSheets(ctx); err != nil {
		return fmt.Errorf("init sheets: %w", err)
	}
	if err := a.initLLM(); err != nil {
		return fmt.Errorf("init llm: %w", err)
	}
	if err := a.initWordPress(); err != nil {
		return fmt.Errorf("init wordpress: %w", err)
	}
	a.initDataForSEO()
	a.initChecker()
	if err := a.initServer(); err != nil {
		return fmt.Errorf("init server: %w", err)
	}

	a.log.Info("contentflow started")
	return nil
}

func (a *Application) Wait(ctx context.Context, cancel context.CancelFunc) error {
	defer cancel()
	<-ctx.Done()
	a.log.Info("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := a.srv.Shutdown(shutdownCtx); err != nil {
		a.log.Error("http shutdown", "err", err)
	}

	a.repo.Close()
	a.wg.Wait()
	a.log.Info("graceful shutdown completed")
	return nil
}

func (a *Application) initConfig() error {
	cfg, err := config.NewConfig()
	if err != nil {
		return err
	}
	a.cfg = cfg
	return nil
}

func (a *Application) initRepo(ctx context.Context) error {
	r, err := repo.New(ctx, a.cfg.Database.URL)
	if err != nil {
		return err
	}
	a.repo = r
	return nil
}

func (a *Application) initSheets(ctx context.Context) error {
	if a.cfg.Sheets.CredentialsFile == "" || a.cfg.Sheets.SpreadsheetID == "" {
		a.log.Warn("sheets not configured — using in-memory mock (no cluster lookup)")
		a.sh = sheets.NewMock()
		return nil
	}
	c, err := sheets.New(ctx, a.cfg.Sheets, a.log)
	if err != nil {
		return err
	}
	a.sh = c
	a.log.Info("sheets configured", "spreadsheet_id", a.cfg.Sheets.SpreadsheetID, "sheet", a.cfg.Sheets.Sheet)
	return nil
}

func (a *Application) initLLM() error {
	client, err := llm.New(a.cfg.LLM.Provider, a.cfg.LLM.APIKey, a.cfg.LLM.Model, a.log)
	if err != nil {
		return err
	}
	a.llm = client
	return nil
}

func (a *Application) initWordPress() error {
	a.wp = wordpress.New(a.cfg.WordPress)
	return nil
}

func (a *Application) initDataForSEO() {
	cfg := a.cfg.DataForSEO
	if cfg.Login == "" || cfg.Password == "" {
		a.log.Warn("dataforseo not configured — using mock SERP data")
		a.serp = dataforseo.NewMock()
		return
	}
	a.serp = dataforseo.New(cfg.Login, cfg.Password)
	a.log.Info("dataforseo configured", "login", cfg.Login, "serp_limit", cfg.SERPLimit)
}

func (a *Application) initChecker() {
	cfg := a.cfg.Checker
	switch cfg.Provider {
	case "mock", "":
		a.log.Warn("checker not configured — using mock (always passes)")
		a.checker = checker.NewMock(cfg.AIThreshold)
	case "huggingface":
		if cfg.APIKey == "" {
			a.log.Warn("huggingface checker missing api key — falling back to mock")
			a.checker = checker.NewMock(cfg.AIThreshold)
			return
		}
		a.checker = huggingface.New(cfg.APIKey, cfg.Model, cfg.AIThreshold, a.log)
		a.log.Info("checker configured", "provider", "huggingface", "model", cfg.Model, "ai_threshold", cfg.AIThreshold)
	default:
		a.log.Warn("unknown checker provider, falling back to mock", "provider", cfg.Provider)
		a.checker = checker.NewMock(cfg.AIThreshold)
	}
}

func (a *Application) initServer() error {
	a.srv = server.New(a.repo, a.processKeyword, a.publishArticle, a.log, server.Config{
		Addr:         a.cfg.Server.Addr,
		ReadTimeout:  a.cfg.Server.ReadTimeout,
		WriteTimeout: a.cfg.Server.WriteTimeout,
	})

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.log.Error("http server", "err", err)
		}
	}()

	return nil
}
