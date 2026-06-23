package llm

import (
	"strings"

	"multiagent-seo/pkg/config"
)

type TaskKind string

const (
	TaskQualify  TaskKind = "qualify"
	TaskBacklink TaskKind = "backlink"
)

func DefaultsFor(c config.LLMConfig, task TaskKind) (provider, model string) {
	switch task {
	case TaskQualify:
		return resolveTaskDefaults(c, c.QualifyProvider, c.QualifyModel)
	case TaskBacklink:
		return resolveTaskDefaults(c, c.BacklinkProvider, c.BacklinkModel)
	}
	return c.Provider, c.Model
}

func resolveTaskDefaults(c config.LLMConfig, taskProvider, taskModel string) (provider, model string) {
	provider = pickStr(taskProvider, c.Provider)
	model = taskModel
	if model != "" {
		return provider, model
	}
	if normalizeProvider(provider) != normalizeProvider(c.Provider) {
		return provider, ModelFor(c, provider)
	}
	return provider, c.Model
}

func ModelFor(c config.LLMConfig, provider string) string {
	switch normalizeProvider(provider) {
	case "groq":
		return c.GroqDefaultModel
	case "claude":
		return c.ClaudeDefaultModel
	}
	if normalizeProvider(provider) == normalizeProvider(c.Provider) {
		return c.Model
	}
	return ""
}

func KeyFor(c config.LLMConfig, provider string) string {
	p := normalizeProvider(provider)
	switch p {
	case "groq":
		if c.GroqAPIKey != "" {
			return c.GroqAPIKey
		}
	case "claude":
		if c.ClaudeAPIKey != "" {
			return c.ClaudeAPIKey
		}
	}
	if p == normalizeProvider(c.Provider) {
		return c.APIKey
	}
	return ""
}

func pickStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func normalizeProvider(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "anthropic" {
		return "claude"
	}
	return s
}
