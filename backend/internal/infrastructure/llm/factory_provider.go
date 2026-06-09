package llm

import (
	"log/slog"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/pkg/config"
)

type Factory struct {
	cfg config.LLMConfig
	log *slog.Logger
}

func NewFactory(cfg config.LLMConfig, log *slog.Logger) *Factory {
	return &Factory{cfg: cfg, log: log}
}

func (f *Factory) ForModel(provider, model string) (articles.LLMClient, error) {
	if model == "" {
		model = f.cfg.ModelFor(provider)
	}
	return New(provider, f.cfg.KeyFor(provider), model, f.log)
}
