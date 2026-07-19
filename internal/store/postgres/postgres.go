// Package postgres provides Postgres-backed implementations of riskguard's
// storage interfaces (ProfileStore, HistoryStore, Blacklist, CounterStore),
// suitable for multi-instance production deployments where the in-memory
// store (internal/store/memory) would not share state across replicas.
package postgres

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/peymanahmadi/riskguard/pkg/riskguard"
)

//go:embed migrations/0001_init.sql
var initSchema string

// Store implements riskguard.ProfileStore, riskguard.HistoryStore,
// riskguard.Blacklist and riskguard.CounterStore backed by a pgxpool.Pool.
// A single pooled connection is shared across all four interfaces since
// they're typically used together and pooling connections is what makes
// this viable under high transaction load.
type Store struct {
	pool *pgxpool.Pool
}

// New opens a connection pool to dsn (e.g.
// "postgres://user:pass@localhost:5432/riskguard") and returns a ready-to-use
// Store. Callers own the returned Store and must call Close when done.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Migrate applies the embedded schema. It's idempotent (CREATE TABLE IF NOT
// EXISTS) and intended for local development / demo use; production
// deployments should use a real migration tool driven from CI/CD instead.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, initSchema)
	if err != nil {
		return fmt.Errorf("postgres: migrate: %w", err)
	}
	return nil
}

func (s *Store) Close() {
	s.pool.Close()
}

// --- riskguard.ProfileStore ---

func (s *Store) GetProfile(ctx context.Context, entityID string) (riskguard.Profile, error) {
	var p riskguard.Profile
	var lastSeenAt *time.Time
	p.EntityID = entityID

	row := s.pool.QueryRow(ctx, `
		SELECT home_country, last_country, last_lat, last_lon, last_seen_at,
		       average_amount, transactions_sum, transactions_cnt
		FROM entity_profiles WHERE entity_id = $1`, entityID)

	err := row.Scan(&p.HomeCountry, &p.LastCountry, &p.LastLat, &p.LastLon, &lastSeenAt,
		&p.AverageAmount, &p.TransactionsSum, &p.TransactionsCnt)
	if err == pgx.ErrNoRows {
		return riskguard.Profile{EntityID: entityID}, nil
	}
	if err != nil {
		return riskguard.Profile{}, fmt.Errorf("postgres: get profile: %w", err)
	}
	if lastSeenAt != nil {
		p.LastSeenAt = *lastSeenAt
	}

	rows, err := s.pool.Query(ctx, `SELECT device_id FROM entity_devices WHERE entity_id = $1`, entityID)
	if err != nil {
		return riskguard.Profile{}, fmt.Errorf("postgres: get devices: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var deviceID string
		if err := rows.Scan(&deviceID); err != nil {
			return riskguard.Profile{}, fmt.Errorf("postgres: scan device: %w", err)
		}
		p.KnownDeviceIDs = append(p.KnownDeviceIDs, deviceID)
	}
	return p, rows.Err()
}

func (s *Store) SaveProfile(ctx context.Context, p riskguard.Profile) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO entity_profiles
			(entity_id, home_country, last_country, last_lat, last_lon, last_seen_at,
			 average_amount, transactions_sum, transactions_cnt)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (entity_id) DO UPDATE SET
			home_country = EXCLUDED.home_country,
			last_country = EXCLUDED.last_country,
			last_lat = EXCLUDED.last_lat,
			last_lon = EXCLUDED.last_lon,
			last_seen_at = EXCLUDED.last_seen_at,
			average_amount = EXCLUDED.average_amount,
			transactions_sum = EXCLUDED.transactions_sum,
			transactions_cnt = EXCLUDED.transactions_cnt
	`, p.EntityID, p.HomeCountry, p.LastCountry, p.LastLat, p.LastLon, nullableTime(p.LastSeenAt),
		p.AverageAmount, p.TransactionsSum, p.TransactionsCnt)
	if err != nil {
		return fmt.Errorf("postgres: save profile: %w", err)
	}

	for _, deviceID := range p.KnownDeviceIDs {
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO entity_devices (entity_id, device_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, p.EntityID, deviceID); err != nil {
			return fmt.Errorf("postgres: save device: %w", err)
		}
	}
	return nil
}

func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// --- riskguard.HistoryStore ---

func (s *Store) RecentTransactions(ctx context.Context, entityID string, since time.Time) ([]riskguard.Transaction, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, entity_id, merchant_id, amount_minor, currency, ip, device_id,
		       country, lat, lon, payment_method, created_at, metadata
		FROM transactions
		WHERE entity_id = $1 AND created_at > $2
		ORDER BY created_at DESC`, entityID, since)
	if err != nil {
		return nil, fmt.Errorf("postgres: recent transactions: %w", err)
	}
	defer rows.Close()

	var out []riskguard.Transaction
	for rows.Next() {
		var tx riskguard.Transaction
		var metadataRaw []byte
		if err := rows.Scan(&tx.ID, &tx.EntityID, &tx.MerchantID, &tx.AmountMinor, &tx.Currency,
			&tx.IP, &tx.DeviceID, &tx.Country, &tx.Lat, &tx.Lon, &tx.PaymentMethod,
			&tx.CreatedAt, &metadataRaw); err != nil {
			return nil, fmt.Errorf("postgres: scan transaction: %w", err)
		}
		if len(metadataRaw) > 0 {
			if err := json.Unmarshal(metadataRaw, &tx.Metadata); err != nil {
				return nil, fmt.Errorf("postgres: unmarshal metadata: %w", err)
			}
		}
		out = append(out, tx)
	}
	return out, rows.Err()
}

func (s *Store) SaveTransaction(ctx context.Context, tx riskguard.Transaction) error {
	metadataRaw, err := json.Marshal(tx.Metadata)
	if err != nil {
		return fmt.Errorf("postgres: marshal metadata: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO transactions
			(id, entity_id, merchant_id, amount_minor, currency, ip, device_id,
			 country, lat, lon, payment_method, created_at, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (id) DO NOTHING
	`, tx.ID, tx.EntityID, tx.MerchantID, tx.AmountMinor, tx.Currency, tx.IP, tx.DeviceID,
		tx.Country, tx.Lat, tx.Lon, tx.PaymentMethod, tx.CreatedAt, metadataRaw)
	if err != nil {
		return fmt.Errorf("postgres: save transaction: %w", err)
	}
	return nil
}

// --- riskguard.Blacklist ---

func (s *Store) IsBlacklisted(ctx context.Context, kind, value string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM blacklist WHERE kind = $1 AND value = $2)`,
		kind, value).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("postgres: blacklist check: %w", err)
	}
	return exists, nil
}

func (s *Store) AddToBlacklist(ctx context.Context, kind, value, reason string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO blacklist (kind, value, reason) VALUES ($1, $2, $3)
		ON CONFLICT (kind, value) DO UPDATE SET reason = EXCLUDED.reason`,
		kind, value, reason)
	if err != nil {
		return fmt.Errorf("postgres: add to blacklist: %w", err)
	}
	return nil
}

// --- riskguard.CounterStore ---

// Increment records one event for key and returns the count of events for
// that key within the trailing window. Stale rows beyond the window are
// opportunistically cleaned up on the same call to keep the table bounded
// without needing a separate cron job.
func (s *Store) Increment(ctx context.Context, key string, window time.Duration) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("postgres: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `INSERT INTO counter_events (key, occurred_at) VALUES ($1, now())`, key); err != nil {
		return 0, fmt.Errorf("postgres: insert counter event: %w", err)
	}

	var count int64
	cutoff := time.Now().Add(-window)
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM counter_events WHERE key = $1 AND occurred_at > $2`,
		key, cutoff).Scan(&count); err != nil {
		return 0, fmt.Errorf("postgres: count events: %w", err)
	}

	// Best-effort cleanup of this key's stale events; safe to skip on error.
	_, _ = tx.Exec(ctx, `DELETE FROM counter_events WHERE key = $1 AND occurred_at <= $2`, key, cutoff)

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("postgres: commit: %w", err)
	}
	return count, nil
}
