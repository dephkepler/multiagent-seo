package application

import (
	"context"
	"fmt"
	"time"

	"contentflow/internal/checker"
	"contentflow/internal/dataforseo"
	"contentflow/internal/prompt"
	"contentflow/internal/wordpress"
)

// StepUsage holds token counts for one LLM call.
type StepUsage struct {
	Step         string `json:"step"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

// TokenSummary is the token usage breakdown returned in the API response.
type TokenSummary struct {
	Steps        []StepUsage `json:"steps"`
	TotalInput   int         `json:"total_input"`
	TotalOutput  int         `json:"total_output"`
}

// generateOutput carries the article content plus pipeline metadata back to the caller.
type generateOutput struct {
	content        string
	competitorData any
	checkResult    any
	checkCycles    any
	humanized      bool
	checkPassed    bool
	provider       string
	model          string
	tokenSummary   *TokenSummary
	durationMS     int64
}

// CycleRecord tracks one check+rewrite iteration.
type CycleRecord struct {
	Cycle       int             `json:"cycle"`
	Threshold   float64         `json:"threshold"`
	CheckResult *checker.Result `json:"check_result"`
}

type checkOutput struct {
	result     *checker.Result
	cycles     []CycleRecord
	humanized  bool
	stepUsages []StepUsage
}

func (a *Application) generate(ctx context.Context, articleID int64, job resolvedJob) (generateOutput, error) {
	start := time.Now()
	var steps []StepUsage

	complete := func(step, p string) (string, error) {
		content, u, err := job.client.Complete(ctx, p, job.maxTokens)
		if err != nil {
			return "", err
		}
		steps = append(steps, StepUsage{
			Step:         step,
			InputTokens:  u.InputTokens,
			OutputTokens: u.OutputTokens,
		})
		return content, nil
	}

	// Step 1: fetch top competitors from SERP.
	a.log.Info("step 1/5: serp competitors — fetching top results",
		"keyword", job.keyword,
		"limit", a.cfg.DataForSEO.SERPLimit,
	)
	serpData, err := a.serp.GetSERP(ctx, job.keyword, job.language, a.cfg.DataForSEO.SERPLimit)
	if err != nil {
		a.log.Warn("serp lookup failed, continuing without competitor data", "err", err)
		serpData, _ = dataforseo.NewMock().GetSERP(ctx, job.keyword, job.language, 0)
	} else {
		a.log.Info("serp competitors loaded",
			"count", len(serpData.Results),
			"keyword", job.keyword,
		)
		if saveErr := a.repo.SaveCompetitorData(ctx, articleID, serpData); saveErr != nil {
			a.log.Warn("save competitor data", "err", saveErr)
		}
	}

	competitors := prompt.Competitors{}
	for _, r := range serpData.Results {
		competitors.Items = append(competitors.Items, prompt.CompetitorItem{
			Rank:        r.Rank,
			Title:       r.Title,
			Description: r.Description,
		})
	}

	// Step 2: brief.
	a.log.Info("step 2/5: brief — building with competitor data + keywords",
		"keyword", job.keyword,
		"competitors", len(competitors.Items),
		"target_keywords", len(job.cluster.Keywords),
		"provider", job.provider,
		"model", job.model,
	)
	brief, err := complete("brief", prompt.Brief(job.keyword, job.language, job.siteTopic, job.extraRules, competitors))
	if err != nil {
		return generateOutput{}, fmt.Errorf("brief: %w", err)
	}

	// Step 3: write full article.
	a.log.Info("step 3/5: writing — full article from brief",
		"keyword", job.keyword,
		"min_words", job.minWords,
		"max_words", job.maxWords,
	)
	article, err := complete("writer", prompt.Writer(brief, job.keyword, job.language, job.minWords, job.maxWords, job.cluster))
	if err != nil {
		return generateOutput{}, fmt.Errorf("writer: %w", err)
	}

	// Step 4: editor pass.
	a.log.Info("step 4/5: editing — SEO polish pass", "keyword", job.keyword)
	edited, err := complete("editor", prompt.Editor(article, job.keyword, job.minWords, job.maxWords, job.cluster))
	if err != nil {
		return generateOutput{}, fmt.Errorf("editor: %w", err)
	}

	// Step 5: originality check + humanize loop.
	a.log.Info("step 5/5: originality check — AI detection + plagiarism", "keyword", job.keyword)
	edited, checkOut := a.checkAndHumanize(ctx, articleID, job, edited)
	steps = append(steps, checkOut.stepUsages...)

	postID, editURL, err := a.wp.CreateDraft(ctx, wordpress.Post{
		Title:   job.keyword,
		Content: edited,
	})
	if err != nil {
		return generateOutput{}, fmt.Errorf("wordpress draft: %w", err)
	}

	if err := a.repo.UpdateDraft(ctx, articleID, postID, editURL); err != nil {
		return generateOutput{}, fmt.Errorf("update db: %w", err)
	}

	checkPassed := checkOut.result == nil || checkOut.result.Original

	summary := buildSummary(steps)
	durationMS := time.Since(start).Milliseconds()

	a.log.Info("generation done",
		"keyword", job.keyword,
		"post_id", postID,
		"check_passed", checkPassed,
		"duration_ms", durationMS,
		"total_input_tokens", summary.TotalInput,
		"total_output_tokens", summary.TotalOutput,
	)

	return generateOutput{
		content:        edited,
		competitorData: serpData,
		checkResult:    checkOut.result,
		checkCycles:    checkOut.cycles,
		humanized:      checkOut.humanized,
		checkPassed:    checkPassed,
		provider:       job.provider,
		model:          job.model,
		tokenSummary:   summary,
		durationMS:     durationMS,
	}, nil
}

func (a *Application) checkAndHumanize(ctx context.Context, articleID int64, job resolvedJob, content string) (string, checkOutput) {
	maxCycles := job.maxCycles
	if maxCycles <= 0 {
		maxCycles = 3
	}
	threshold := job.aiThreshold
	if threshold == 0 {
		threshold = a.cfg.Checker.AIThreshold
	}

	var (
		cycles     []CycleRecord
		stepUsages []StepUsage
	)

	for cycle := 1; cycle <= maxCycles; cycle++ {
		checkRes, err := a.checker.Check(ctx, content)
		if err != nil {
			a.log.Warn("originality check failed, skipping", "cycle", cycle, "err", err)
			break
		}

		passes := checkRes.AIScore < threshold
		checkRes.Original = passes

		a.log.Info("check result",
			"cycle", cycle,
			"ai_score", checkRes.AIScore,
			"threshold", threshold,
			"passes", passes,
			"provider", checkRes.Provider,
		)

		cycles = append(cycles, CycleRecord{Cycle: cycle, Threshold: threshold, CheckResult: checkRes})

		if saveErr := a.repo.SaveCheckResult(ctx, articleID, checkRes); saveErr != nil {
			a.log.Warn("save check result", "err", saveErr)
		}

		if passes {
			return content, checkOutput{
				result:     checkRes,
				cycles:     cycles,
				humanized:  cycle > 1,
				stepUsages: stepUsages,
			}
		}

		if cycle == maxCycles {
			a.log.Warn("max humanize cycles reached, publishing as-is",
				"cycles", maxCycles,
				"final_ai_score", checkRes.AIScore,
			)
			break
		}

		a.log.Info("content flagged — humanize rewrite",
			"cycle", cycle,
			"ai_score", checkRes.AIScore,
			"sentences_flagged", len(checkRes.SentencesFlagged),
		)

		humanizedContent, u, err := job.client.Complete(
			ctx,
			prompt.Humanize(content, job.keyword, checkRes.Issues, checkRes.SentencesFlagged),
			job.maxTokens,
		)
		if err != nil {
			a.log.Warn("humanize step failed, using current content", "cycle", cycle, "err", err)
			break
		}
		stepUsages = append(stepUsages, StepUsage{
			Step:         fmt.Sprintf("humanize_%d", cycle),
			InputTokens:  u.InputTokens,
			OutputTokens: u.OutputTokens,
		})
		content = humanizedContent
	}

	var lastResult *checker.Result
	if len(cycles) > 0 {
		lastResult = cycles[len(cycles)-1].CheckResult
	}

	return content, checkOutput{
		result:     lastResult,
		cycles:     cycles,
		humanized:  false,
		stepUsages: stepUsages,
	}
}

func buildSummary(steps []StepUsage) *TokenSummary {
	s := &TokenSummary{Steps: steps}
	for _, u := range steps {
		s.TotalInput += u.InputTokens
		s.TotalOutput += u.OutputTokens
	}
	return s
}

