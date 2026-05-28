package checker

import (
	"context"
	"fmt"
	"log/slog"

	"multiagent-seo/internal/domain/generate"
	"multiagent-seo/internal/infrastructure/checker/huggingface"
)

// checkFunc abstracts over the concrete provider so the adapter only depends on
// "run a check, hand back the common result fields". This also keeps the
// checker package from caring whether the provider's result is checker.Result
// or huggingface.Result.
type checkFunc func(ctx context.Context, content string) (*generate.CheckResult, error)

type adapter struct {
	check checkFunc
}

var _ generate.ContentChecker = (*adapter)(nil)

// New builds a generate.ContentChecker for the given provider. Supported
// providers are "mock" (and "" as an alias) and "huggingface".
//
// "originality" is rejected explicitly: the legacy config comment listed it as
// an option but no such provider was ever implemented, so callers get a clear
// error instead of a silent mock fallback.
func New(provider, apiKey, model string, threshold float64, log *slog.Logger) (generate.ContentChecker, error) {
	switch provider {
	case "mock", "":
		mock := NewMock(threshold)
		return &adapter{check: func(ctx context.Context, content string) (*generate.CheckResult, error) {
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
		return &adapter{check: func(ctx context.Context, content string) (*generate.CheckResult, error) {
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

func (a *adapter) Check(ctx context.Context, content string) (*generate.CheckResult, error) {
	return a.check(ctx, content)
}

func toCheckResult(ai, plag float64, original bool, provider string, issues, flagged []string, reportURL string) *generate.CheckResult {
	return &generate.CheckResult{
		AIScore:          ai,
		PlagiarismScore:  plag,
		Original:         original,
		Provider:         provider,
		Issues:           issues,
		SentencesFlagged: flagged,
		ReportURL:        reportURL,
	}
}
