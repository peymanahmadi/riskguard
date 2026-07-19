package rules

import (
	"context"
	"fmt"

	"github.com/peymanahmadi/riskguard/pkg/riskguard"
)

// NewDeviceRule flags transactions from a device the entity has never used
// before. On its own this is a fairly weak signal (everyone gets a new
// phone eventually), so its base score is modest — but it's designed to be
// combined with amount/velocity signals via a WeightedScorer or
// SumCappedScorer so that "new device + large amount" compounds correctly.
type NewDeviceRule struct {
	Store riskguard.ProfileStore
	// BaseScore is the score reported when the device is new. Defaults to
	// 25 if zero.
	BaseScore float64
}

func NewNewDeviceRule(store riskguard.ProfileStore) *NewDeviceRule {
	return &NewDeviceRule{Store: store}
}

func (r *NewDeviceRule) Name() string { return "new_device" }

func (r *NewDeviceRule) Evaluate(ctx context.Context, tx riskguard.Transaction) (riskguard.RuleResult, error) {
	if tx.DeviceID == "" {
		return riskguard.RuleResult{Rule: r.Name(), Score: 0, Triggered: false}, nil
	}

	profile, err := r.Store.GetProfile(ctx, tx.EntityID)
	if err != nil {
		return riskguard.RuleResult{}, fmt.Errorf("new_device: get profile: %w", err)
	}

	if len(profile.KnownDeviceIDs) == 0 {
		// First transaction ever seen for this entity; not enough history
		// to call any device "new".
		return riskguard.RuleResult{Rule: r.Name(), Score: 0, Triggered: false}, nil
	}

	if profile.KnowsDevice(tx.DeviceID) {
		return riskguard.RuleResult{Rule: r.Name(), Score: 0, Triggered: false}, nil
	}

	base := r.BaseScore
	if base == 0 {
		base = 25
	}

	return riskguard.RuleResult{
		Rule:      r.Name(),
		Triggered: true,
		Score:     base,
		Severity:  riskguard.SeverityLow,
		Reason:    fmt.Sprintf("device %q not previously seen for entity %s", tx.DeviceID, tx.EntityID),
	}, nil
}
