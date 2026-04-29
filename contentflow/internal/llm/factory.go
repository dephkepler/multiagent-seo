package llm

import (
	"fmt"
	"log/slog"
	"strings"

	"contentflow/internal/llm/claude"
	"contentflow/internal/llm/groq"
)

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
		return groq.New(apiKey, model, log), nil
	case "claude", "anthropic":
		return claude.New(apiKey, model, log), nil
	default:
		return nil, fmt.Errorf("llm: unknown provider %q (supported: groq, claude)", provider)
	}
}
