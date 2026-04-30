package llm

import (
	"fmt"
	"log/slog"
	"strings"

	"contentflow/internal/llm/claude"
	"contentflow/internal/llm/groq"
)

// Groq — free tier with per-minute token limits.
var groqModels = map[string]bool{
	"llama-3.3-70b-versatile": true, // best for SEO: smart + fast, free
	"llama-3.1-70b-versatile": true, // slightly older, similar quality, free
	"llama-3.1-8b-instant":    true, // fastest and cheapest, lower quality
	"mixtral-8x7b-32768":      true, // large context (32k), good for long articles
	"gemma2-9b-it":            true, // Google model, experimental
}

// Claude (Anthropic) — paid, billed per token.
var claudeModels = map[string]bool{
	"claude-haiku-4-5-20251001": true, // cheapest and fastest (~$0.25/1M input tokens)
	"claude-sonnet-4-6":         true, // best price/quality balance (~$3/1M input tokens)
	"claude-opus-4-7":           true, // highest quality, most expensive (~$15/1M input tokens)
}

func New(provider, apiKey, model string, log *slog.Logger) (Client, error) {
	if log == nil {
		log = slog.Default()
	}
	if apiKey == "" {
		return nil, fmt.Errorf("llm: api key is empty for provider %q", provider)
	}
	if model == "" {
		return nil, fmt.Errorf("llm: model is empty for provider %q", provider)
	}

	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "groq":
		if !groqModels[model] {
			return nil, fmt.Errorf("llm: unknown groq model %q (supported: llama-3.3-70b-versatile, llama-3.1-8b-instant, llama-3.1-70b-versatile, mixtral-8x7b-32768, gemma2-9b-it)", model)
		}
		return groq.New(apiKey, model, log), nil
	case "claude", "anthropic":
		if !claudeModels[model] {
			return nil, fmt.Errorf("llm: unknown claude model %q (supported: claude-haiku-4-5-20251001, claude-sonnet-4-6, claude-opus-4-7)", model)
		}
		return claude.New(apiKey, model, log), nil
	default:
		return nil, fmt.Errorf("llm: unknown provider %q (supported: groq, claude)", provider)
	}
}
