package riskguard

import (
	"context"
	"testing"
	"time"
)

// BenchmarkEngine_Evaluate measures end-to-end throughput of Evaluate with a
// handful of lightweight rules, simulating realistic per-rule latency (e.g.
// a fast in-process check plus a couple of ~200us store round-trips) to show
// that concurrent rule evaluation, not just raw CPU, is what the engine is
// optimizing for.
func BenchmarkEngine_Evaluate(b *testing.B) {
	fast := rule("fast", 0, false)
	storeBacked := RuleFunc{FuncName: "store", Fn: func(ctx context.Context, tx Transaction) (RuleResult, error) {
		time.Sleep(200 * time.Microsecond)
		return RuleResult{Rule: "store", Score: 0}, nil
	}}

	e := NewEngine(WithRules(fast, storeBacked, storeBacked, storeBacked))
	tx := Transaction{ID: "bench"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = e.Evaluate(context.Background(), tx)
	}
}
