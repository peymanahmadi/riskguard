# riskguard

[![CI](https://github.com/peymanahmadi/riskguard/actions/workflows/ci.yml/badge.svg)](https://github.com/peymanahmadi/riskguard/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/peymanahmadi/riskguard.svg)](https://pkg.go.dev/github.com/peymanahmadi/riskguard/pkg/riskguard)
[![Go Report Card](https://goreportcard.com/badge/github.com/peymanahmadi/riskguard)](https://goreportcard.com/report/github.com/peymanahmadi/riskguard)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**riskguard is a Go library** for detecting fraud and abuse in payment
transactions in real time: velocity limits, impossible-travel detection,
amount thresholds, new-device checks, and blacklists, combined
concurrently into a single risk score and decision (`approve`, `review`,
or `decline`).

```go
import "github.com/peymanahmadi/riskguard/pkg/riskguard"
```

## What this repository actually is

This repo contains two things, and they are not peers.

1. **`pkg/riskguard`, the library.** This is the deliverable. It has
   **zero external dependencies**, a small interface-driven API, and is
   what you'd `go get` and import into your own service.
2. **Everything else (`cmd/server`, `internal/*`), a reference
   integration.** It exists to prove the library works under real
   conditions (HTTP, Postgres, Kafka, concurrent load) and to give you a
   working example to read. It's demo/test scaffolding, not something
   you're meant to depend on or deploy as-is. None of it is even
   importable outside this module, since it lives under `internal/` and
   Go enforces that boundary.

If you only care about using riskguard in your own project, "Install" and
"Quick start" below are all you need.

## Install

```bash
go get github.com/peymanahmadi/riskguard/pkg/riskguard
```

Requires Go 1.22+. No other dependencies. `pkg/riskguard` imports only the
standard library.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/peymanahmadi/riskguard/pkg/riskguard"
	"github.com/peymanahmadi/riskguard/pkg/riskguard/rules"
)

func main() {
	// Bring your own storage: anything satisfying CounterStore,
	// ProfileStore, or Blacklist works (Redis, DynamoDB, Postgres, an
	// in-memory fake for tests, see "Storage" below).
	counters := myCounterStore{}
	profiles := myProfileStore{}
	blacklist := myBlacklist{}

	engine := riskguard.NewEngine(
		riskguard.WithRules(
			rules.NewVelocityRule(counters, 5*time.Minute, 10),
			rules.NewAmountThresholdRule(50000, "USD"), // $500.00
			rules.NewGeoVelocityRule(profiles),
			rules.NewNewDeviceRule(profiles),
			rules.NewBlacklistRule(blacklist),
		),
		riskguard.WithScorer(riskguard.WeightedScorer{
			Weights: map[string]float64{"blacklist": 4, "geo_velocity": 3},
		}),
		riskguard.WithThresholds(riskguard.Thresholds{Review: 40, Decline: 75}),
		riskguard.WithTimeout(2*time.Second),
	)

	verdict, err := engine.Evaluate(context.Background(), riskguard.Transaction{
		ID: "tx_123", EntityID: "cust_1", AmountMinor: 75000, Currency: "USD",
		IP: "9.9.9.9", DeviceID: "phone-1", CreatedAt: time.Now(),
	})
	if err != nil {
		// Evaluate can return a usable Verdict and a non-nil error at the
		// same time under FailOpen, see "Failure handling" below.
	}

	fmt.Println(verdict.Decision)  // Approve, Review, or Decline
	fmt.Println(verdict.Score)     // 0-100
	fmt.Println(verdict.Reasons()) // human-readable reasons for any triggered rule
}
```

## Why this shape

Payment-risk systems have a few recurring requirements, and they shaped
the design of the library.

- **Low latency under load.** `Engine.Evaluate` runs every rule
  concurrently instead of sequentially. Total latency tracks the slowest
  rule, not the sum of all of them. See
  [`docs/architecture.md`](docs/architecture.md) for the mechanics and a
  throughput benchmark.
- **Explainability.** Every `Verdict` carries the individual
  `RuleResult`s that produced it, not just a final number.
  `Verdict.Reasons()` gives you a human-readable audit trail for support
  agents and compliance.
- **Resilience to partial failure.** A single flaky rule, one whose store
  is slow or down, shouldn't be able to hang or crash evaluation for
  every other rule. The engine recovers from panics, enforces a per-call
  timeout, and lets you choose how to treat rule errors (see below).
- **Storage agnostic.** Counters, profiles, history, and blacklists are
  each a small interface. Bring Redis, DynamoDB, Postgres, or a
  single-process in-memory map. The engine and rules never know or care.

## Public API

The surface you build against lives in `pkg/riskguard` and
`pkg/riskguard/rules`.

| Type / function | Purpose |
|---|---|
| `riskguard.Transaction` | The input: a payment transaction to evaluate. |
| `riskguard.Engine` / `NewEngine(opts...)` | Runs configured rules concurrently and produces a `Verdict`. |
| `riskguard.Rule` | Interface for a single risk check. Implement this for custom rules. |
| `riskguard.Scorer` | Interface for aggregating `[]RuleResult` into one score. Built-ins: `MaxScorer`, `WeightedScorer`, `SumCappedScorer`. |
| `riskguard.Verdict` | The output: `Score`, `Decision`, `Results`, `Reasons()`. |
| `riskguard.Thresholds` | Score cutoffs mapping onto `Approve`, `Review`, `Decline`. |
| `riskguard.CounterStore` | Sliding-window event counts (velocity-style rules). |
| `riskguard.ProfileStore` | Known devices, last location, running averages per entity. |
| `riskguard.HistoryStore` | Recent transaction history per entity. |
| `riskguard.Blacklist` | IP, device, or entity denylist checks. |

Full generated docs: https://pkg.go.dev/github.com/peymanahmadi/riskguard/pkg/riskguard

### Built-in rules (`pkg/riskguard/rules`)

| Rule | Detects |
|---|---|
| `VelocityRule` | Too many transactions from one entity in a sliding time window (card testing, bot abuse). |
| `AmountThresholdRule` | Transaction amount exceeding a configured threshold, scaled rather than a hard cliff. |
| `GeoVelocityRule` | Impossible travel, where the location changes faster than is physically plausible (haversine distance divided by elapsed time). |
| `NewDeviceRule` | Transaction from a device not previously seen for this entity. |
| `BlacklistRule` | IP, device, or entity present in a blacklist. |

### Failure handling

`Engine.Evaluate` can return a non-nil `error` and a usable `Verdict` at
the same time. That isn't a bug, it's how partial rule failure gets
surfaced. Configure the behavior you want via `WithFailurePolicy`:

- `FailOpen` (default): score using whichever rules succeeded. The error
  tells you coverage was degraded, but it doesn't block the decision.
- `FailClosed`: any rule error forces `Review`, no matter what the
  successful rules concluded. Use this when missing signal is itself a
  risk.

### Writing a custom rule

```go
type Rule interface {
	Name() string
	Evaluate(ctx context.Context, tx Transaction) (RuleResult, error)
}
```

Implement those two methods and pass your rule to `riskguard.WithRules(...)`
alongside (or instead of) the built-ins. No changes to the engine are
needed. See `pkg/riskguard/rules/*.go` for five real examples, or
`riskguard.RuleFunc` if you'd rather wrap a plain function than define a
named type.

## Reference integration (secondary, for proving it works)

The rest of the repo wires the library into an HTTP API, a Postgres-backed
storage implementation, and a Kafka consumer/producer pipeline, so the
library gets exercised under real concurrent load and not just unit
tests. If you're evaluating this project rather than just using the
library, this is the part worth reading to see the design decisions in
context. Start with [`docs/architecture.md`](docs/architecture.md).

```
pkg/riskguard/            the library, start here
pkg/riskguard/rules/      built-in rules
cmd/server/                demo binary: wires the library to HTTP + Postgres + Kafka
internal/store/postgres/  example ProfileStore/HistoryStore/Blacklist/CounterStore backed by Postgres
internal/store/memory/    in-memory implementations (used by the demo and by rule unit tests)
internal/kafka/           example event-driven pipeline (transactions in, decisions out)
internal/api/             minimal HTTP handler for the demo server
test/integration/         tests against real Postgres/Kafka (docker compose)
docs/architecture.md       design rationale, sequence diagrams, why rules run concurrently
```

Running the demo locally:

```bash
go run ./cmd/server                                    # in-memory stores, no infra needed
# or, for the full stack:
make up      # docker compose up -d --build (postgres:16-alpine + kafka 8.2.2 + the server)
make demo    # fires a sample transaction at the HTTP API
make down
```

## Verified end to end

This isn't just "it builds." It's been run against real infrastructure.

- **Unit tests**: 80.7% coverage on `pkg/riskguard` and
  `pkg/riskguard/rules` (`make cover`), race-detector clean in CI.
- **Integration tests** (`make integration-test`): pass against live
  Postgres and Kafka containers. Profile round trips, sliding-window
  counters, and a Kafka produce/consume round trip.
- **Concurrent-load benchmark**: `BenchmarkEngine_Evaluate` comes in
  around 580µs/op, 16 allocs/op, evaluating 5 rules concurrently
  (`make bench`).
- **Manual load test**: firing 15 rapid requests for the same entity
  correctly triggers `VelocityRule` starting at request 11 (limit 10),
  with the score climbing predictably as the window fills. That confirms
  the sliding-window counter and weighted scoring behave as designed
  under real repeated calls, not just mocked ones.

## Known limitations

- `go test -race` requires cgo, which isn't enabled by default on
  Windows. Use CI (Linux runners) for race detection, or install a GCC
  toolchain (MSYS2/mingw) or use WSL2 locally.
- The in-memory `CounterStore`/`ProfileStore` (`internal/store/memory`)
  are single-process only. That's fine for the demo and for unit tests,
  but a multi-instance deployment needs a shared backend: the included
  Postgres implementation, or bring your own Redis/DynamoDB one.
- `AmountThresholdRule` currently only judges amounts already in its
  configured currency. Multi-currency support means either composing one
  instance per currency or converting to a common currency upstream.

## Versioning

Tagged releases follow semver. See [CHANGELOG.md](CHANGELOG.md) for what
changed in each. The library lives at module path
`github.com/peymanahmadi/riskguard`, and you import
`github.com/peymanahmadi/riskguard/pkg/riskguard` and
`github.com/peymanahmadi/riskguard/pkg/riskguard/rules`.

## License

MIT. See [LICENSE](LICENSE).