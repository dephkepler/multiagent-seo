package application

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"contentflow/internal/checker"
	"contentflow/internal/dataforseo"
	"contentflow/internal/prompt"
	"contentflow/internal/publisher"
)

type StepUsage struct {
	Step         string `json:"step"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

type TokenSummary struct {
	Steps       []StepUsage `json:"steps"`
	TotalInput  int         `json:"total_input"`
	TotalOutput int         `json:"total_output"`
}

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

func (a *Application) generate(ctx context.Context, log *slog.Logger, articleID int64, job resolvedJob) (generateOutput, error) {
	start := time.Now()
	var steps []StepUsage

	complete := func(step, p string) (string, error) {
		stepStart := time.Now()
		content, u, err := job.client.Complete(ctx, p, job.maxTokens)
		if err != nil {
			return "", err
		}
		log.Debug("llm step done",
			"step", step,
			"duration_ms", time.Since(stepStart).Milliseconds(),
			"input_tokens", u.InputTokens,
			"output_tokens", u.OutputTokens,
		)
		steps = append(steps, StepUsage{
			Step:         step,
			InputTokens:  u.InputTokens,
			OutputTokens: u.OutputTokens,
		})
		return content, nil
	}

	log.Info("step 1/5: serp competitors — fetching top results",
		"limit", a.cfg.DataForSEO.SERPLimit,
	)
	serpData, err := a.serp.GetSERP(ctx, job.keyword, job.language, a.cfg.DataForSEO.SERPLimit)
	if err != nil {
		log.Warn("serp lookup failed, continuing without competitor data", "err", err)
		serpData, _ = dataforseo.NewMock().GetSERP(ctx, job.keyword, job.language, 0)
	} else {
		log.Info("serp competitors loaded",
			"count", len(serpData.Results),
		)
		if saveErr := a.repo.SaveCompetitorData(ctx, articleID, serpData); saveErr != nil {
			log.Warn("save competitor data", "err", saveErr)
		}
	}

	competitors := prompt.Competitors{}
	for _, r := range serpData.Results {
		competitors.Items = append(competitors.Items, prompt.CompetitorItem{
			Rank:        r.Rank,
			URL:         r.URL,
			Title:       r.Title,
			Description: r.Description,
		})
	}
	competitors.PAA = append(competitors.PAA, serpData.PAA...)
	if serpData.FeaturedSnippet != nil {
		competitors.FeaturedSnippet = &prompt.FeaturedSnippetItem{
			Title:       serpData.FeaturedSnippet.Title,
			Description: serpData.FeaturedSnippet.Description,
		}
	}

	log.Info("step 2/5: brief — building with competitor data + keywords",
		"competitors", len(competitors.Items),
		"target_keywords", len(job.cluster.Keywords),
	)
	brief, err := complete("brief", prompt.Brief(job.keyword, job.language, job.cluster, job.siteTopic, job.extraRules, competitors))
	if err != nil {
		return generateOutput{}, fmt.Errorf("brief: %w", err)
	}

	log.Info("step 3/5: writing — full article from brief",
		"min_words", job.minWords,
		"max_words", job.maxWords,
	)
	article, err := complete("writer", prompt.Writer(brief, job.keyword, job.language, job.minWords, job.maxWords, job.cluster, competitors))
	if err != nil {
		return generateOutput{}, fmt.Errorf("writer: %w", err)
	}

	log.Info("step 4/5: editing — SEO polish pass")
	edited, err := complete("editor", prompt.Editor(article, job.keyword, job.minWords, job.maxWords, job.cluster, competitors))
	if err != nil {
		return generateOutput{}, fmt.Errorf("editor: %w", err)
	}

	log.Info("step 5/5: originality check — AI detection + plagiarism")
	edited, checkOut := a.checkAndHumanize(ctx, log, articleID, job, edited)
	steps = append(steps, checkOut.stepUsages...)

	var imgResolver publisher.ImageResolver
	if job.includeImages {
		imgResolver = publisher.NewPexelsResolver(a.pexels)
	}
	body, renderStats := publisher.RenderHTML(ctx, edited, publisher.RenderOptions{
		Keyword:     job.keyword,
		Resolver:    imgResolver,
		Attribution: job.imageAttribution,
	})
	postID, editURL, err := job.publisher.CreateDraft(ctx, publisher.Post{
		Title:   job.keyword,
		Content: body,
	})
	if err != nil {
		return generateOutput{}, fmt.Errorf("publisher draft: %w", err)
	}

	if err := a.repo.UpdateDraft(ctx, articleID, postID, editURL); err != nil {
		return generateOutput{}, fmt.Errorf("update db: %w", err)
	}
	if err := a.repo.SaveImageStats(ctx, articleID, renderStats.ImagesRequested, renderStats.ImagesResolved, renderStats.ImagesSkipped); err != nil {
		log.Warn("save image stats", "err", err)
	}

	checkPassed := checkOut.result == nil || checkOut.result.Original

	summary := buildSummary(steps)
	durationMS := time.Since(start).Milliseconds()

	log.Info("generation done",
		"post_id", postID,
		"check_passed", checkPassed,
		"duration_ms", durationMS,
		"total_input_tokens", summary.TotalInput,
		"total_output_tokens", summary.TotalOutput,
		"images_requested", renderStats.ImagesRequested,
		"images_resolved", renderStats.ImagesResolved,
		"images_skipped", renderStats.ImagesSkipped,
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

func (a *Application) checkAndHumanize(ctx context.Context, log *slog.Logger, articleID int64, job resolvedJob, content string) (string, checkOutput) {
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
			log.Warn("originality check failed, skipping", "cycle", cycle, "err", err)
			break
		}

		passes := checkRes.AIScore < threshold
		checkRes.Original = passes

		log.Info("check result",
			"cycle", cycle,
			"ai_score", checkRes.AIScore,
			"threshold", threshold,
			"passes", passes,
			"provider", checkRes.Provider,
		)

		cycles = append(cycles, CycleRecord{Cycle: cycle, Threshold: threshold, CheckResult: checkRes})

		if saveErr := a.repo.SaveCheckResult(ctx, articleID, checkRes); saveErr != nil {
			log.Warn("save check result", "err", saveErr)
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
			log.Warn("max humanize cycles reached, publishing as-is",
				"cycles", maxCycles,
				"final_ai_score", checkRes.AIScore,
			)
			break
		}

		log.Info("content flagged — humanize rewrite",
			"cycle", cycle,
			"ai_score", checkRes.AIScore,
			"sentences_flagged", len(checkRes.SentencesFlagged),
		)

		step := fmt.Sprintf("humanize_%d", cycle)
		stepStart := time.Now()
		humanizedContent, u, err := job.client.Complete(
			ctx,
			prompt.Humanize(content, job.keyword, checkRes.Issues, checkRes.SentencesFlagged),
			job.maxTokens,
		)
		if err != nil {
			log.Warn("humanize step failed, using current content", "cycle", cycle, "err", err)
			break
		}
		log.Debug("llm step done",
			"step", step,
			"duration_ms", time.Since(stepStart).Milliseconds(),
			"input_tokens", u.InputTokens,
			"output_tokens", u.OutputTokens,
		)
		stepUsages = append(stepUsages, StepUsage{
			Step:         step,
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
