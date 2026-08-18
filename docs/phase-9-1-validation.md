# Phase 9.1 validation

Status: core implementation approved; broad release gate remains open.

## Implemented

- Optional versioned daily, weekly, or monthly reward-eligibility policies.
- Household-local boundaries with snapshotted timezone and week-start settings.
- Daily zero-grace enforcement and weekly/monthly 0, 12, 24, or 48-hour grace.
- Minimum net approved points, optional completion percentage, and optional redemption cap.
- Lazy, idempotent immutable period evaluation with reversal adjustments.
- Separate period score and lifetime signed point balance.
- Atomic redemption enforcement for evaluation, cap, reward cost, and current balance.
- Safe first-policy and frequency-transition behavior; ordinary rule changes preserve the preceding period under its old version.
- Parent settings/progress/history and child collecting, awaiting, eligible, and not-eligible states.

## Automated evidence

- Fresh PostgreSQL 16 migrations 1–15: pass.
- Uncached database-backed rewards, HTTP, database, points, and habits suites: pass.
- Policy replacement, frequency transition, same-calendar version transition, exact-balance redemption, and cap tests: pass.
- Backend full Go tests, `go vet ./...`, API build, and migrator build: pass.
- OpenAPI YAML parses: 48 paths, 473 references, no missing schema reference.
- Frontend Prettier, ESLint, TypeScript, 62/62 serialized Vitest tests, and production build: pass.
- Strict implementation audit: approved with no critical/high correctness, security, privacy, concurrency, or data-integrity blocker.

## Release evidence still open

- Cursor pagination for evaluation history.
- Immutable qualifying award/reversal ID snapshots for deeper evaluation explainability.
- Concrete parent calendar preview for collection, cutoff, and redemption dates.
- Production scheduler/worker in addition to correctness-preserving lazy evaluation.
- Full DST, timezone, grace, cutoff-race, hostile-write, authorization, and concurrent-request matrix.
- Authenticated browser/axe journey, manual assistive-technology/device/reflow checks, final scans, SBOMs, and deployment digests.

These items keep the broad Phase 9.1 testing checkbox and production gate open. They are not known critical or high defects in the implemented core.
