package rules_test

import (
	"context"
	"testing"

	"github.com/peymanahmadi/riskguard/pkg/riskguard"
	"github.com/peymanahmadi/riskguard/pkg/riskguard/rules"
)

func TestAmountThresholdRule(t *testing.T) {
	rule := rules.NewAmountThresholdRule(10000, "USD") // $100.00 threshold

	tests := []struct {
		name      string
		amount    int64
		currency  string
		triggered bool
	}{
		{"below threshold", 5000, "USD", false},
		{"at threshold", 10000, "USD", false},
		{"just above", 10100, "USD", true},
		{"far above, should cap at 100", 1000000, "USD", true},
		{"different currency ignored", 1000000, "EUR", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := riskguard.Transaction{AmountMinor: tt.amount, Currency: tt.currency}
			res, err := rule.Evaluate(context.Background(), tx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Triggered != tt.triggered {
				t.Fatalf("triggered = %v, want %v (score %v)", res.Triggered, tt.triggered, res.Score)
			}
			if res.Score < 0 || res.Score > 100 {
				t.Fatalf("score out of bounds: %v", res.Score)
			}
		})
	}
}
