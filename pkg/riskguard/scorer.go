package riskguard

// Scorer aggregates the individual RuleResults produced for one transaction
// into a single risk score in the range [0, 100].
type Scorer interface {
	Aggregate(results []RuleResult) float64
}

// MaxScorer takes the highest individual rule score. Good default when any
// single strong signal (e.g. a blacklist hit) should be able to drive the
// decision on its own, regardless of what other rules say.
type MaxScorer struct{}

func (MaxScorer) Aggregate(results []RuleResult) float64 {
	var max float64
	for _, r := range results {
		if r.Score > max {
			max = r.Score
		}
	}
	return clamp(max)
}

// WeightedScorer computes a weighted average of rule scores. Rules not
// present in Weights default to a weight of 1.0. Unlike MaxScorer, several
// medium-confidence signals can combine to cross the decline threshold even
// if no single rule does.
type WeightedScorer struct {
	Weights map[string]float64
}

func (w WeightedScorer) Aggregate(results []RuleResult) float64 {
	if len(results) == 0 {
		return 0
	}
	var sumScore, sumWeight float64
	for _, r := range results {
		weight := 1.0
		if w.Weights != nil {
			if wt, ok := w.Weights[r.Rule]; ok {
				weight = wt
			}
		}
		sumScore += r.Score * weight
		sumWeight += weight
	}
	if sumWeight == 0 {
		return 0
	}
	return clamp(sumScore / sumWeight)
}

// SumCappedScorer sums all triggered rule scores, capped at 100. This
// rewards "many small signals" more aggressively than WeightedScorer, at the
// cost of being easier to push into false positives if rules overlap.
type SumCappedScorer struct{}

func (SumCappedScorer) Aggregate(results []RuleResult) float64 {
	var sum float64
	for _, r := range results {
		if r.Triggered {
			sum += r.Score
		}
	}
	return clamp(sum)
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
