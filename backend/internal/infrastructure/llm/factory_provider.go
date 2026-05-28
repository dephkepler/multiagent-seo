package llm

import (
	"log/slog"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/pkg/config"
)

var _ articles.LLMFactory = (*Factory)(nil)

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
	return New(provider, f.cfg.KeyFor(provider), model, f.log)
}
