package riskguard

import (
	"context"
	"time"
)

// CounterStore tracks sliding-window event counts, used for velocity-style
// rules (e.g. "no more than N transactions per 5 minutes"). Implementations
// must be safe for concurrent use and should expire entries older than the
// requested window on their own (e.g. via TTL in Redis, or periodic sweep in
// memory).
type CounterStore interface {
	// Increment records one event for key and returns the number of events
	// recorded for that key within the trailing window.
	Increment(ctx context.Context, key string, window time.Duration) (count int64, err error)
}

// ProfileStore provides the known-good historical profile for an entity
// (customer/account), used by rules like new-device or geo-velocity
// detection.
type ProfileStore interface {
	GetProfile(ctx context.Context, entityID string) (Profile, error)
	// SaveProfile persists an updated profile, e.g. after observing a new
	// device or location. Implementations may choose to do this
	// asynchronously.
	SaveProfile(ctx context.Context, profile Profile) error
}

// HistoryStore provides access to recent transactions for an entity, used by
// rules that need more than the profile snapshot (e.g. sum of transactions
// in the last 24h).
type HistoryStore interface {
	RecentTransactions(ctx context.Context, entityID string, since time.Time) ([]Transaction, error)
	SaveTransaction(ctx context.Context, tx Transaction) error
}

// Blacklist checks arbitrary (kind, value) pairs — e.g. ("ip", "1.2.3.4") or
// ("device", "abc123") — against a list of known-bad values.
type Blacklist interface {
	IsBlacklisted(ctx context.Context, kind, value string) (bool, error)
}
