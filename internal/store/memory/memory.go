// Package memory provides in-memory implementations of riskguard's storage
// interfaces. They're safe for concurrent use and are intended for tests,
// local development, and the demo service — not for multi-instance
// production deployments (use the postgres/redis-backed stores for that).
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/peymanahmadi/riskguard/pkg/riskguard"
)

// CounterStore is a sliding-window counter backed by a per-key slice of
// timestamps. Good enough for demo/test throughput; a production system
// with high cardinality keys should prefer Redis (INCR + EXPIRE, or a sorted
// set for exact sliding windows).
type CounterStore struct {
	mu     sync.Mutex
	events map[string][]time.Time
	now    func() time.Time // overridable for tests
}

func NewCounterStore() *CounterStore {
	return &CounterStore{events: make(map[string][]time.Time), now: time.Now}
}

func (s *CounterStore) Increment(_ context.Context, key string, window time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	cutoff := now.Add(-window)

	events := s.events[key]
	kept := events[:0]
	for _, t := range events {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	s.events[key] = kept

	return int64(len(kept)), nil
}

// ProfileStore is a simple map-backed ProfileStore guarded by a RWMutex.
type ProfileStore struct {
	mu       sync.RWMutex
	profiles map[string]riskguard.Profile
}

func NewProfileStore() *ProfileStore {
	return &ProfileStore{profiles: make(map[string]riskguard.Profile)}
}

func (s *ProfileStore) GetProfile(_ context.Context, entityID string) (riskguard.Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.profiles[entityID]
	if !ok {
		return riskguard.Profile{EntityID: entityID}, nil
	}
	return p, nil
}

func (s *ProfileStore) SaveProfile(_ context.Context, profile riskguard.Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles[profile.EntityID] = profile
	return nil
}

// HistoryStore is a simple append-only, map-backed transaction history.
type HistoryStore struct {
	mu   sync.RWMutex
	byID map[string][]riskguard.Transaction
}

func NewHistoryStore() *HistoryStore {
	return &HistoryStore{byID: make(map[string][]riskguard.Transaction)}
}

func (s *HistoryStore) RecentTransactions(_ context.Context, entityID string, since time.Time) ([]riskguard.Transaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := s.byID[entityID]
	out := make([]riskguard.Transaction, 0, len(all))
	for _, tx := range all {
		if tx.CreatedAt.After(since) {
			out = append(out, tx)
		}
	}
	return out, nil
}

func (s *HistoryStore) SaveTransaction(_ context.Context, tx riskguard.Transaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[tx.EntityID] = append(s.byID[tx.EntityID], tx)
	return nil
}

// Blacklist is a simple set-backed Blacklist guarded by a RWMutex.
type Blacklist struct {
	mu      sync.RWMutex
	entries map[string]struct{}
}

func NewBlacklist(seed ...string) *Blacklist {
	b := &Blacklist{entries: make(map[string]struct{})}
	for _, s := range seed {
		b.entries[s] = struct{}{}
	}
	return b
}

func (b *Blacklist) Add(kind, value string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries[kind+":"+value] = struct{}{}
}

func (b *Blacklist) IsBlacklisted(_ context.Context, kind, value string) (bool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.entries[kind+":"+value]
	return ok, nil
}
