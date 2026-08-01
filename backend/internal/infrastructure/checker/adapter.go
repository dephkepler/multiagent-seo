package checker

import (
	"fmt"
	"log/slog"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/infrastructure/checker/copyleaks"
	"multiagent-seo/internal/infrastructure/checker/huggingface"
)

type Config struct {
	Provider    string
	Email       string
	APIKey      string
	Model       string
	AIThreshold float64
	Sandbox     bool
}

func New(cfg Config, log *slog.Logger) (articles.ContentChecker, error) {
	switch cfg.Provider {
	case "mock", "":
		return NewMock(cfg.AIThreshold), nil
	case "huggingface":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("checker: huggingface provider requires an API key")
		}
		return huggingface.New(cfg.APIKey, cfg.Model, cfg.AIThreshold, log), nil
	case "copyleaks":
		if cfg.Email == "" || cfg.APIKey == "" {
			return nil, fmt.Errorf("checker: copyleaks provider requires email and API key")
		}
		return copyleaks.New(cfg.Email, cfg.APIKey, cfg.AIThreshold, cfg.Sandbox, log), nil
	default:
		return nil, fmt.Errorf("checker: unknown provider %q", cfg.Provider)
	}
}
