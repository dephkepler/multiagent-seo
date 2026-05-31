package checker

import (
	"context"
	"fmt"
	"log/slog"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/infrastructure/checker/huggingface"
)

// checkFunc lets the adapter stay agnostic about whether the provider returns
// checker.Result or huggingface.Result; both get mapped to articles.CheckResult.
type checkFunc func(ctx context.Context, content string) (*articles.CheckResult, error)

type adapter struct {
	check checkFunc
}

// New builds a articles.ContentChecker for the given provider: "mock" (or ""),
// or "huggingface". "originality" errors explicitly rather than silently falling
// back to the mock — it was advertised in legacy config but never implemented.
func New(provider, apiKey, model string, threshold float64, log *slog.Logger) (articles.ContentChecker, error) {
	switch provider {
	case "mock", "":
		mock := NewMock(threshold)
		return &adapter{check: func(ctx context.Context, content string) (*articles.CheckResult, error) {
			res, err := mock.Check(ctx, content)
			if err != nil {
				return nil, err
			}
			return toCheckResult(res.AIScore, res.PlagiarismScore, res.Original, res.Provider,
				res.Issues, res.SentencesFlagged, res.ReportURL), nil
		}}, nil
	case "huggingface":
		if apiKey == "" {
			return nil, fmt.Errorf("checker: huggingface provider requires an API key")
		}
		hf := huggingface.New(apiKey, model, threshold, log)
		return &adapter{check: func(ctx context.Context, content string) (*articles.CheckResult, error) {
			res, err := hf.Check(ctx, content)
			if err != nil {
				return nil, err
			}
			return toCheckResult(res.AIScore, res.PlagiarismScore, res.Original, res.Provider,
				res.Issues, res.SentencesFlagged, res.ReportURL), nil
		}}, nil
	case "originality":
		return nil, fmt.Errorf("checker: provider %q is not implemented", provider)
	default:
		return nil, fmt.Errorf("checker: unknown provider %q", provider)
	}
}

func (a *adapter) Check(ctx context.Context, content string) (*articles.CheckResult, error) {
	return a.check(ctx, content)
}

func toCheckResult(ai, plag float64, original bool, provider string, issues, flagged []string, reportURL string) *articles.CheckResult {
	return &articles.CheckResult{
		AIScore:          ai,
		PlagiarismScore:  plag,
		Original:         original,
		Provider:         provider,
		Issues:           issues,
		SentencesFlagged: flagged,
		ReportURL:        reportURL,
	}
}
