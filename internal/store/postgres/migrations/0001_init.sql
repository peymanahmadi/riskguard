-- Schema for the riskguard demo service.
-- Applied automatically by cmd/server on startup for local/dev use;
-- in a real deployment this would run through a proper migration tool
-- (golang-migrate, atlas, etc.) as part of CI/CD.

CREATE TABLE IF NOT EXISTS entity_profiles (
    entity_id        TEXT PRIMARY KEY,
    home_country     TEXT NOT NULL DEFAULT '',
    last_country     TEXT NOT NULL DEFAULT '',
    last_lat         DOUBLE PRECISION NOT NULL DEFAULT 0,
    last_lon         DOUBLE PRECISION NOT NULL DEFAULT 0,
    last_seen_at     TIMESTAMPTZ,
    average_amount   BIGINT NOT NULL DEFAULT 0,
    transactions_sum BIGINT NOT NULL DEFAULT 0,
    transactions_cnt BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS entity_devices (
    entity_id TEXT NOT NULL REFERENCES entity_profiles(entity_id) ON DELETE CASCADE,
    device_id TEXT NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (entity_id, device_id)
);

CREATE TABLE IF NOT EXISTS transactions (
    id             TEXT PRIMARY KEY,
    entity_id      TEXT NOT NULL,
    merchant_id    TEXT NOT NULL DEFAULT '',
    amount_minor   BIGINT NOT NULL,
    currency       TEXT NOT NULL,
    ip             TEXT NOT NULL DEFAULT '',
    device_id      TEXT NOT NULL DEFAULT '',
    country        TEXT NOT NULL DEFAULT '',
    lat            DOUBLE PRECISION NOT NULL DEFAULT 0,
    lon            DOUBLE PRECISION NOT NULL DEFAULT 0,
    payment_method TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL,
    metadata       JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_transactions_entity_created ON transactions (entity_id, created_at DESC);

CREATE TABLE IF NOT EXISTS blacklist (
    kind  TEXT NOT NULL,
    value TEXT NOT NULL,
    added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reason TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (kind, value)
);

-- Generic sliding-window event log backing the Postgres CounterStore
-- implementation (velocity rules etc). A background job or periodic
-- DELETE (see cleanup query in postgres.go) keeps this table bounded.
CREATE TABLE IF NOT EXISTS counter_events (
    key         TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_counter_events_key_time ON counter_events (key, occurred_at DESC);
