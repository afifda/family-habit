# Phase 5 validation

Phase 5 implements parent-managed recurring habits, effective-dated child assignments, one-off tasks, and deterministic occurrence generation.

## Delivered guarantees

- Habit presentation and assignments are effective-dated; edits apply to “this and future”.
- Multi-child assignment creation is atomic and each child receives a distinct occurrence.
- One-off tasks create their occurrence transactionally and remain neutrally overdue until completed or cancelled.
- Cancellation requires a reason and can atomically finalize a pending completion attempt.
- Occurrences snapshot title, description, icon, color, type, points, and dates. Progressed or attempted history is immutable.
- Effective edits and deactivation remove only untouched future recurring occurrences; the generator recreates canonical rows without duplicates.
- Every Phase 5 mutation supports idempotent replay. Updates and deletes accept an expected resource version through `If-Match`.
- Generation uses household-local dates and converges under concurrent calls.

## Verification evidence

- PostgreSQL migrations 1–10 applied from an empty database and on the development stack.
- PostgreSQL integration tests cover concurrent idempotency, stale versions, pending cancellation, protected history, snapshot regeneration, weekday boundaries, a DST transition, concurrent family generation, deactivation, overlap prevention, and atomic archived-child rejection.
- `go test ./...`, `go vet ./...`, and builds for `cmd/api` and `cmd/migrate` pass.
- Frontend formatting, ESLint, TypeScript, 23 Vitest tests, and the Vite production build pass.
- The OpenAPI document parses and its local references resolve.
- Docker Compose rebuilt all application images; PostgreSQL, API, and frontend are healthy, migrations exited successfully, and `/health/ready` returned `{"status":"ready"}`.

## Implementation notes

- Occurrences are generated lazily by `EnsureDate(family, localDate)`; Phase 6 will invoke this from the Child Today endpoint.
- The frontend derives form dates from the household timezone and retries a failed atomic assignment without creating a duplicate habit definition.
- The API remains designed for the current single-instance VPS deployment; the existing process-local authentication/PIN rate-limit assumption is unchanged.
