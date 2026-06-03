package llm

import (
	"log/slog"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/pkg/config"
)

// Factory resolves an LLMClient per provider/model, supplying the API key from
// config (KeyFor handles the groq/claude/anthropic fallback).
type Factory struct {
	cfg config.LLMConfig
	log *slog.Logger
}

func NewFactory(cfg config.LLMConfig, log *slog.Logger) *Factory {
	return &Factory{cfg: cfg, log: log}
}

func (f *Factory) ForModel(provider, model string) (articles.LLMClient, error) {
	// Empty model: pick the provider's own default, so callers that only know
	// the vendor (e.g. /generate with provider=claude) don't accidentally ship
	// a model name belonging to a different provider.
	if model == "" {
		model = f.cfg.ModelFor(provider)
	}
	return New(provider, f.cfg.KeyFor(provider), model, f.log)
}
