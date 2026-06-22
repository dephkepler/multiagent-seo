package fakes

import (
	"context"
	"time"

	"multiagent-seo/internal/domain/articles"
)

type RewardUpdate struct {
	ArticleID int64
	Stage     string
	Human     *float64
	Weight    float64
}

type PromptStore struct {
	Stats      []articles.PromptVariantStat
	StatsErr   error
	Failures   []articles.PromptFailure
	FailErr    error
	InsertErr  error
	Inserted   []articles.PromptVariant
	Statuses   map[int64]articles.VariantStatus
	LastReward *RewardUpdate
}

func (f *PromptStore) ActiveVariants(context.Context, string) ([]articles.PromptVariant, error) {
	return nil, nil
}

func (f *PromptStore) InsertVariant(_ context.Context, v articles.PromptVariant) (int64, error) {
	if f.InsertErr != nil {
		return 0, f.InsertErr
	}
	f.Inserted = append(f.Inserted, v)
	return int64(len(f.Inserted)), nil
}

func (f *PromptStore) SetVariantStatus(_ context.Context, id int64, st articles.VariantStatus) error {
	if f.Statuses == nil {
		f.Statuses = map[int64]articles.VariantStatus{}
	}
	f.Statuses[id] = st
	return nil
}

func (f *PromptStore) SaveOutcome(context.Context, articles.PromptOutcome) error { return nil }

func (f *PromptStore) SelectionStats(context.Context, string, time.Time) ([]articles.PromptVariantStat, error) {
	return f.Stats, f.StatsErr
}

func (f *PromptStore) WorstOutcomes(context.Context, int64, time.Time, int) ([]articles.PromptFailure, error) {
	return f.Failures, f.FailErr
}

func (f *PromptStore) UpdateOutcomeReward(_ context.Context, articleID int64, stage string, human *float64, weight float64) error {
	f.LastReward = &RewardUpdate{articleID, stage, human, weight}
	return nil
}
