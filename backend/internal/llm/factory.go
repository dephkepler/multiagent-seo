package llm

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"multiagent-seo/internal/llm/claude"
	"multiagent-seo/internal/llm/groq"
)

// No model-name validation: unknown models surface as a 4xx from the provider.
var providers = map[string]func(apiKey, model string, log *slog.Logger) Client{
	"groq":      func(k, m string, l *slog.Logger) Client { return groq.New(k, m, l) },
	"claude":    func(k, m string, l *slog.Logger) Client { return claude.New(k, m, l) },
	"anthropic": func(k, m string, l *slog.Logger) Client { return claude.New(k, m, l) },
}

func New(provider, apiKey, model string, log *slog.Logger) (Client, error) {
	if log == nil {
		log = slog.Default()
	}
	name := strings.ToLower(strings.TrimSpace(provider))
	if name == "" {
		return nil, fmt.Errorf("llm: provider is empty (supported: %s)", supportedProviders())
	}
	ctor, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("llm: unknown provider %q (supported: %s)", provider, supportedProviders())
	}
	if apiKey == "" {
		return nil, fmt.Errorf("llm: api key is empty for provider %q", provider)
	}
	if model == "" {
		return nil, fmt.Errorf("llm: model is empty for provider %q", provider)
	}
	return ctor(apiKey, model, log), nil
}

// Sorted so error messages are deterministic across map-iteration order.
func supportedProviders() string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
