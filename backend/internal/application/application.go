package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"multiagent-seo/internal/checker"
	"multiagent-seo/internal/checker/huggingface"
	"multiagent-seo/internal/config"
	"multiagent-seo/internal/dataforseo"
	"multiagent-seo/internal/llm"
	"multiagent-seo/internal/pexels"
	"multiagent-seo/internal/publisher"
	"multiagent-seo/internal/repo"
	"multiagent-seo/internal/server"
	"multiagent-seo/internal/sheets"
	"multiagent-seo/internal/wordpress"
)

type Application struct {
	cfg        *config.Config
	log        *slog.Logger
	repo       *repo.Repo
	llm        llm.Client
	publishers map[string]publisher.Publisher
	sh         sheets.Client
	serp       dataforseo.Client
	checker    checker.Client
	pexels     *pexels.Client
	srv        *server.Server
	wg         sync.WaitGroup

	// bgCancel aborts in-flight generations on shutdown so SIGTERM doesn't
	// wait out the per-job timeout.
	bgCtx    context.Context
	bgCancel context.CancelFunc
}

func New(log *slog.Logger) *Application {
	return &Application{log: log}
}

func (a *Application) Start(ctx context.Context) error {
	if err := a.initConfig(); err != nil {
		return fmt.Errorf("init config: %w", err)
	}
	// Derived from ctx so parent cancellation (SIGTERM) propagates to background work.
	a.bgCtx, a.bgCancel = context.WithCancel(ctx)

	if err := a.initRepo(ctx); err != nil {
		return fmt.Errorf("init repo: %w", err)
	}
	if err := a.initSheets(ctx); err != nil {
		return fmt.Errorf("init sheets: %w", err)
	}
	if err := a.initLLM(); err != nil {
		return fmt.Errorf("init llm: %w", err)
	}
	if err := a.initPublishers(); err != nil {
		return fmt.Errorf("init publishers: %w", err)
	}
	a.initDataForSEO()
	a.initChecker()
	a.initPexels()
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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), a.cfg.Server.ShutdownWaitTimeout)
	defer shutdownCancel()

	// Order matters: HTTP first (no new jobs), then cancel bg ctx, then drain,
	// then close the pool — earlier pool close would race with MarkFailed/UpdateDraft.
	if err := a.srv.Shutdown(shutdownCtx); err != nil {
		a.log.Error("http shutdown", "err", err)
	}

	if a.bgCancel != nil {
		a.bgCancel()
	}

	if err := waitWithCtx(&a.wg, shutdownCtx); err != nil {
		a.log.Warn("background jobs did not drain in time, closing pool anyway",
			"err", err,
			"timeout", a.cfg.Server.ShutdownWaitTimeout,
		)
	}

	a.repo.Close()
	a.log.Info("graceful shutdown completed")
	return nil
}

// waitWithCtx returns nil when wg drains, ctx.Err() when ctx fires first.
func waitWithCtx(wg *sync.WaitGroup, ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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

// initPublishers builds one Publisher per site; the alias key lets a
// non-WordPress backend drop in later without touching callers.
func (a *Application) initPublishers() error {
	a.publishers = make(map[string]publisher.Publisher, len(a.cfg.WordPress))
	for _, alias := range a.cfg.SiteAliases() {
		siteLog := a.log.With("publisher", "wordpress", "site", alias)
		a.publishers[alias] = wordpress.New(a.cfg.WordPress[alias], siteLog)
		a.log.Info("publisher configured",
			"site", alias,
			"kind", "wordpress",
			"url", a.cfg.WordPress[alias].URL,
		)
	}
	return nil
}

// publisherFor falls back to the "default" alias when site is empty.
func (a *Application) publisherFor(site string) (publisher.Publisher, bool) {
	if site == "" {
		site = config.DefaultSite
	}
	p, ok := a.publishers[site]
	return p, ok
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

// initPexels leaves a.pexels nil when disabled or unkeyed; RenderHTML then
// strips [IMG | ...] placeholders instead of failing the pipeline.
func (a *Application) initPexels() {
	if !a.cfg.Pexels.Enabled {
		a.log.Info("pexels disabled by flag — [IMG] placeholders will be stripped")
		return
	}
	key := a.cfg.Pexels.APIKey
	if key == "" {
		a.log.Warn("pexels enabled but CF_PEXELS_API_KEY empty — [IMG] placeholders will be stripped")
		return
	}
	a.pexels = pexels.New(key)
	a.log.Info("pexels configured")
}

func (a *Application) initServer() error {
	a.srv = server.New(a.repo, a.processKeyword, a.publishArticle, a.log, server.Config{
		Addr:         a.cfg.Server.Addr,
		ReadTimeout:  a.cfg.Server.ReadTimeout,
		WriteTimeout: a.cfg.Server.WriteTimeout,
	})

	a.wg.Go(func() {
		if err := a.srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.log.Error("http server", "err", err)
		}
	})

	return nil
}
