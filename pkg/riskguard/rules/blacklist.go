package rules

import (
	"context"
	"fmt"

	"github.com/peymanahmadi/riskguard/pkg/riskguard"
)

// BlacklistRule checks the transaction's IP and device against a Blacklist
// store. A hit is treated as a near-certain abuse signal and scores close to
// the maximum, deliberately so that even conservative scorers (e.g.
// WeightedScorer) will still push the decision toward decline/review.
type BlacklistRule struct {
	Store riskguard.Blacklist
}

func NewBlacklistRule(store riskguard.Blacklist) *BlacklistRule {
	return &BlacklistRule{Store: store}
}

func (r *BlacklistRule) Name() string { return "blacklist" }

func (r *BlacklistRule) Evaluate(ctx context.Context, tx riskguard.Transaction) (riskguard.RuleResult, error) {
	checks := []struct{ kind, value string }{
		{"ip", tx.IP},
		{"device", tx.DeviceID},
		{"entity", tx.EntityID},
	}

	for _, c := range checks {
		if c.value == "" {
			continue
		}
		hit, err := r.Store.IsBlacklisted(ctx, c.kind, c.value)
		if err != nil {
			return riskguard.RuleResult{}, fmt.Errorf("blacklist: check %s: %w", c.kind, err)
		}
		if hit {
			return riskguard.RuleResult{
				Rule:      r.Name(),
				Triggered: true,
				Score:     100,
				Severity:  riskguard.SeverityCritical,
				Reason:    fmt.Sprintf("%s %q is blacklisted", c.kind, c.value),
			}, nil
		}
	}

	return riskguard.RuleResult{Rule: r.Name(), Score: 0, Triggered: false}, nil
}
