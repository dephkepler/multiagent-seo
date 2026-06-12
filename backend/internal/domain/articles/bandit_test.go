package articles

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestSampleBetaMean(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	const n = 200_000
	a, b := 7.0, 3.0
	sum := 0.0
	for range n {
		sum += sampleBeta(rng, a, b)
	}
	mean := sum / n
	want := a / (a + b)
	if math.Abs(mean-want) > 0.01 {
		t.Errorf("Beta(%.0f,%.0f) mean = %.4f, want ~%.4f", a, b, mean, want)
	}
}

func TestThompsonPickFavorsStronger(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 7))
	stats := []PromptVariantStat{
		{Variant: PromptVariant{ID: 1}, Samples: 100, SumReward: 70, SumComplement: 30},
		{Variant: PromptVariant{ID: 2}, Samples: 100, SumReward: 40, SumComplement: 60},
	}
	wins := map[int64]int{}
	const n = 10_000
	for range n {
		v, ok := ThompsonPick(rng, stats)
		if !ok {
			t.Fatal("ThompsonPick returned no variant")
		}
		wins[v.ID]++
	}
	if wins[1] <= wins[2] {
		t.Errorf("stronger variant should win more often: v1=%d v2=%d", wins[1], wins[2])
	}
}

func TestShouldPromote(t *testing.T) {
	champion := PromptVariantStat{Variant: PromptVariant{ID: 1, Status: VariantChampion}, Samples: 100, SumReward: 50, SumComplement: 50}

	t.Run("promotes clearly better candidate", func(t *testing.T) {
		cand := PromptVariantStat{Variant: PromptVariant{ID: 2, Status: VariantCandidate}, Samples: 100, SumReward: 80, SumComplement: 20}
		p, r, ok := ShouldPromote([]PromptVariantStat{champion, cand}, 30)
		if !ok || p != 2 || r != 1 {
			t.Errorf("got promote=%d retire=%d ok=%v, want 2,1,true", p, r, ok)
		}
	})

	t.Run("skips candidate with too few samples", func(t *testing.T) {
		cand := PromptVariantStat{Variant: PromptVariant{ID: 2, Status: VariantCandidate}, Samples: 5, SumReward: 4, SumComplement: 1}
		if _, _, ok := ShouldPromote([]PromptVariantStat{champion, cand}, 30); ok {
			t.Error("should not promote with too few samples")
		}
	})

	t.Run("skips weaker candidate", func(t *testing.T) {
		cand := PromptVariantStat{Variant: PromptVariant{ID: 2, Status: VariantCandidate}, Samples: 100, SumReward: 30, SumComplement: 70}
		if _, _, ok := ShouldPromote([]PromptVariantStat{champion, cand}, 30); ok {
			t.Error("should not promote weaker candidate")
		}
	})
}

func TestThompsonPickExploresUncertain(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 9))
	stats := []PromptVariantStat{
		{Variant: PromptVariant{ID: 1}, Samples: 1000, SumReward: 700, SumComplement: 300},
		{Variant: PromptVariant{ID: 2}, Samples: 2, SumReward: 1, SumComplement: 1},
	}
	wins := map[int64]int{}
	const n = 10_000
	for range n {
		v, _ := ThompsonPick(rng, stats)
		wins[v.ID]++
	}
	if wins[2] == 0 {
		t.Error("uncertain variant should still get explored sometimes")
	}
}
