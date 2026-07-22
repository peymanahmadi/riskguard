# Changelog

## [v1.0.0] - 2026-07-22

### Changed
- First stable release. No functional changes from v0.1.0, retagged as
  v1.0.0 after fixing a stale module-proxy cache and per pkg.go.dev's
  recommendation to tag a stable major version.

## [v0.1.0] - 2026-07-22

### Added
- Core risk engine with concurrent rule evaluation, panic recovery, per-call timeout, and fail-open/fail-closed policies
- Five fraud-detection rules: velocity, amount threshold, geo-velocity (impossible travel), new-device, blacklist
- Three scoring strategies: max, weighted average, capped sum
- Postgres-backed storage implementation with connection pooling
- Kafka pipeline for asynchronous transaction evaluation
- HTTP demo server
- Full CI pipeline: unit tests with race detector, lint, integration tests against live Postgres and Kafka
