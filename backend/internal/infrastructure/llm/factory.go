package llm

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/infrastructure/llm/claude"
	"multiagent-seo/internal/infrastructure/llm/groq"
	"multiagent-seo/internal/infrastructure/llm/transport"
)

// No model-name validation: unknown models surface as a 4xx from the provider.
var providers = map[string]func(apiKey, model string, log *slog.Logger) *transport.Client{
	"groq":      groq.New,
	"claude":    claude.New,
	"anthropic": claude.New,
}

func New(provider, apiKey, model string, log *slog.Logger) (articles.LLMClient, error) {
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
	return &client{transport: ctor(apiKey, model, log)}, nil
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
