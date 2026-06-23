package promptevolution

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/domain/articles/prompt"
)

const (
	writerSelectionWindow = 30 * 24 * time.Hour
	promoteMinSamples     = 30
	worstSampleSize       = 5
	maxCandidates         = 2
	evolveMaxTokens       = 2000
	canaryMinSamples      = 10
)

type Service struct {
	prompts articles.PromptStore
	llm     articles.LLMFactory
	log     *slog.Logger
}

func NewService(prompts articles.PromptStore, llm articles.LLMFactory, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{prompts: prompts, llm: llm, log: log}
}

func (s *Service) PromotePrompts(ctx context.Context) {
	if s.prompts == nil {
		return
	}
	stage := articles.PromptStageWriter
	stats, err := s.prompts.SelectionStats(ctx, stage, time.Now().Add(-writerSelectionWindow))
	if err != nil {
		s.log.WarnContext(ctx, "promote: load stats failed", "stage", stage, "err", err)
		return
	}
	for _, id := range articles.ShouldRetire(stats, canaryMinSamples) {
		if err := s.prompts.SetVariantStatus(ctx, id, articles.VariantRetired); err != nil {
			s.log.WarnContext(ctx, "canary: retire weak candidate failed", "id", id, "err", err)
		} else {
			s.log.InfoContext(ctx, "canary: retired weak candidate", "id", id)
		}
	}

	promote, retire, ok := articles.ShouldPromote(stats, promoteMinSamples)
	if !ok {
		return
	}
	if retire != 0 {
		if err := s.prompts.SetVariantStatus(ctx, retire, articles.VariantRetired); err != nil {
			s.log.WarnContext(ctx, "promote: retire old champion failed", "id", retire, "err", err)
			return
		}
	}
	if err := s.prompts.SetVariantStatus(ctx, promote, articles.VariantChampion); err != nil {
		s.log.ErrorContext(ctx, "promote: set new champion failed", "id", promote, "err", err)
		return
	}
	s.log.InfoContext(ctx, "promoted prompt variant", "stage", stage, "new_champion", promote, "retired", retire)
}

func (s *Service) GenerateCandidate(ctx context.Context) {
	if s.prompts == nil {
		return
	}
	stage := articles.PromptStageWriter
	since := time.Now().Add(-writerSelectionWindow)

	stats, err := s.prompts.SelectionStats(ctx, stage, since)
	if err != nil {
		s.log.WarnContext(ctx, "evolve: load stats failed", "stage", stage, "err", err)
		return
	}

	var champion *articles.PromptVariant
	candidates := 0
	for i := range stats {
		switch stats[i].Variant.Status {
		case articles.VariantChampion:
			champion = &stats[i].Variant
		case articles.VariantCandidate:
			candidates++
		}
	}
	if champion == nil {
		s.log.WarnContext(ctx, "evolve: no champion, skip")
		return
	}
	if candidates >= maxCandidates {
		s.log.InfoContext(ctx, "evolve: candidate slots full, skip", "candidates", candidates)
		return
	}

	failures, err := s.prompts.WorstOutcomes(ctx, champion.ID, since, worstSampleSize)
	if err != nil {
		s.log.WarnContext(ctx, "evolve: load failures failed", "err", err)
		return
	}
	if len(failures) == 0 {
		s.log.InfoContext(ctx, "evolve: no failures yet, skip")
		return
	}

	frozen, evolving := prompt.SplitWriter(champion.Body)

	client, err := s.llm.ForModel("claude", "")
	if err != nil {
		s.log.WarnContext(ctx, "evolve: claude client failed", "err", err)
		return
	}
	newEvolving, _, err := client.Complete(ctx, prompt.EvolveWriter(evolving, failures), evolveMaxTokens)
	if err != nil {
		s.log.WarnContext(ctx, "evolve: claude completion failed", "err", err)
		return
	}
	newEvolving = strings.TrimSpace(newEvolving)
	if newEvolving == "" {
		s.log.WarnContext(ctx, "evolve: claude returned empty, skip")
		return
	}

	id, err := s.prompts.InsertVariant(ctx, articles.PromptVariant{
		Stage:    stage,
		Body:     prompt.JoinWriter(frozen, newEvolving),
		Status:   articles.VariantCandidate,
		Origin:   articles.OriginGenerated,
		ParentID: &champion.ID,
	})
	if err != nil {
		s.log.WarnContext(ctx, "evolve: insert candidate failed", "err", err)
		return
	}
	s.log.InfoContext(ctx, "evolve: generated new candidate", "id", id, "parent", champion.ID, "failures", len(failures))
}
