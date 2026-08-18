# Phase 9 validation

Status: implementation approved; production release evidence remains open.

## Implemented and verified

- Dynamic household routine groups, ordering, time hints, archival, and an automatic Other group.
- Effective-dated recurring assignment moves and immutable historical occurrence snapshots.
- Optional routine assignment for one-off tasks and recurring child assignments.
- Routine-first Child Today views with workflow filters.
- Off-by-default rewards, parent catalog management, explicit eligibility, and child-safe catalog projections.
- Atomic requested, fulfilled, and cancelled redemption lifecycle with append-only debit/refund entries.
- Serialized child balances, exact debit/refund linkage, nonnegative redemption and approval-reversal guards.
- Separate earned, redeemed, refunded, and net report values.
- Household-, actor-, state-, and child-bound pagination cursors.
- Versioned and idempotent reward-setting changes with exact stored-response replay.
- Structural PostgreSQL guards for tenant actors, redemption lifecycle, selected eligibility, routine ordering, icons, and ledger actors.

## Automated evidence

- Fresh PostgreSQL 16 migrations 1–14: pass.
- Uncached database-backed suites for database, routines, rewards, habits, points, and HTTP API: pass.
- Routine archive compaction with three active groups: pass.
- Cross-filter and cross-projection redemption cursor rejection: pass.
- Household rewards toggle with an intervening mutation and byte-identical idempotent replay: pass.
- Backend full unit suite and `go vet ./...`: pass.
- Frontend Prettier, ESLint, TypeScript, and production build: pass.
- Frontend Vitest suite, serialized to avoid worker resource contention: 59/59 pass.
- Strict implementation re-audit: approved with no critical or high implementation blocker.

## Production release evidence still required

- Authenticated browser journey covering routine management, child completion, reward request, and parent decision.
- Authenticated axe scan and manual keyboard/screen-reader checks.
- 320 px, 200%/400% reflow, forced-colors, representative phone/tablet/desktop checks.
- Final container and dependency scans, SBOMs, and recorded commit/image digests.

These remaining items keep the broad Phase 9 testing checkbox and production gate open; they do not represent known feature or data-integrity defects.
