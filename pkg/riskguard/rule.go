package riskguard

import "context"

// Severity is a coarse classification of how much a triggered rule should
// influence the overall risk score. Concrete scorers may use it directly or
// ignore it in favor of raw Score values.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityLow
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityLow:
		return "low"
	case SeverityMedium:
		return "medium"
	case SeverityHigh:
		return "high"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// RuleResult is the outcome of a single rule evaluating a single
// transaction.
type RuleResult struct {
	Rule      string
	Triggered bool
	// Score is a value in [0, 100] representing how risky this individual
	// rule considers the transaction to be. A non-triggered rule should
	// generally report 0, but is not required to.
	Score    float64
	Severity Severity
	Reason   string
}

// Rule evaluates a single transaction and reports how risky it considers it
// to be. Implementations must be safe for concurrent use: the Engine may
// invoke Evaluate for many transactions, and for many rules on the same
// transaction, concurrently.
//
// A Rule should return an error only when it cannot render a judgment at all
// (e.g. its backing store is unreachable) — not to signal that the
// transaction looks risky. Use RuleResult for that.
type Rule interface {
	Name() string
	Evaluate(ctx context.Context, tx Transaction) (RuleResult, error)
}

// RuleFunc adapts a plain function to the Rule interface, useful for tests
// and small ad-hoc rules.
type RuleFunc struct {
	FuncName string
	Fn       func(ctx context.Context, tx Transaction) (RuleResult, error)
}

func (f RuleFunc) Name() string { return f.FuncName }

func (f RuleFunc) Evaluate(ctx context.Context, tx Transaction) (RuleResult, error) {
	return f.Fn(ctx, tx)
}
