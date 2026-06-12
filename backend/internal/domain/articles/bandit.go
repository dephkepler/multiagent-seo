package articles

import (
	"math"
	"math/rand/v2"
)

func sampleGamma(rng *rand.Rand, shape float64) float64 {
	if shape < 1 {
		u := rng.Float64()
		return sampleGamma(rng, shape+1) * math.Pow(u, 1/shape)
	}
	d := shape - 1.0/3.0
	c := 1.0 / math.Sqrt(9*d)
	for {
		x := rng.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := rng.Float64()
		if u < 1-0.0331*x*x*x*x {
			return d * v
		}
		if math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
}

func sampleBeta(rng *rand.Rand, a, b float64) float64 {
	x := sampleGamma(rng, a) // success
	y := sampleGamma(rng, b) // failure
	if x+y == 0 {
		return 0.5
	}
	return x / (x + y)
}

func ThompsonPick(rng *rand.Rand, variants []PromptVariantStat) (PromptVariant, bool) {
	bestTheta := -1.0
	var pick PromptVariant
	found := false

	for _, v := range variants {
		alfa := 1 + v.SumReward
		beta := 1 + v.SumComplement
		theta := sampleBeta(rng, alfa, beta)
		if theta > bestTheta {
			bestTheta = theta
			pick = v.Variant // capture the variant
			found = true
		}
	}

	return pick, found
}

func variantMean(s PromptVariantStat) float64 {
	a := 1 + s.SumReward
	b := 1 + s.SumComplement
	return a / (a + b)
}

func variantLowerBound(s PromptVariantStat) float64 {
	a := 1 + s.SumReward
	b := 1 + s.SumComplement
	mean := a / (a + b)
	variance := (a * b) / ((a + b) * (a + b) * (a + b + 1))
	lb := mean - 1.2816*math.Sqrt(variance)
	if lb < 0 {
		return 0
	}
	return lb
}

func ShouldPromote(stats []PromptVariantStat, minSamples int) (promote int64, retire int64, ok bool) {
	bar := -1.0
	var champion int64
	for _, s := range stats {
		if s.Variant.Status == VariantChampion {
			bar = variantMean(s)
			champion = s.Variant.ID
		}
	}

	var best int64
	found := false
	for _, s := range stats {
		if s.Variant.Status != VariantCandidate || s.Samples < minSamples {
			continue
		}
		if lb := variantLowerBound(s); lb > bar {
			bar = lb
			best = s.Variant.ID
			found = true
		}
	}
	if !found {
		return 0, 0, false
	}
	return best, champion, true
}
