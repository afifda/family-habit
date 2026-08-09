# Phase 6 validation

Phase 6 delivers the first complete child workflow: viewing household-local work, submitting it for parent review, and withdrawing it while still pending.

## Delivered guarantees

- Today is generated from the household timezone and contains recurring work for that local date plus actionable due-today and overdue one-off tasks.
- A child can read only their own Today projection. Sibling, cross-household, archived, and nonexistent resources are hidden consistently.
- The server supplies stable groups, counts, detail snapshots, due states, resource versions, and permitted actions.
- Submission and withdrawal require CSRF, an opaque idempotency key, and the expected occurrence version.
- Attempt allocation, occurrence transition, audit event, and idempotency response commit atomically under row locks.
- Submission creates no point-ledger entry. Points remain entirely within the Phase 7 parent decision workflow.
- Ambiguous client failures retain and replay the same idempotency key before refreshing canonical state.
- The child dialog manages initial focus, focus containment, Escape, trigger restoration, and destination-group focus after a state transition.

## Verification evidence

- `go test ./...`, `go vet ./...`, and API/migration builds pass.
- PostgreSQL integration tests cover household-local dates, due-today/overdue/future membership, stable grouping, same- and different-key concurrency, submit/cancel and submit/archive races, withdrawal versus a parent-decision transaction, expired/revoked sessions, CSRF no-write behavior, zero ledger writes, and staged transaction rollback.
- Frontend formatting, ESLint, TypeScript, all 35 Vitest tests, and the Vite production build pass. Nine focused tests cover the Phase 6 child experience.
- OpenAPI YAML parses and documents opaque 1–128-character idempotency keys, version preconditions, completion state, and resource-hiding behavior.
- Docker Compose rebuilt successfully; PostgreSQL, API, and frontend are healthy, migrations completed, the frontend is reachable, and `/health/ready` returned `{"status":"ready"}`.
- A final independent product/security/accessibility re-audit approved the release gate after two remediation passes.

## Phase 7 handoff

Phase 7 must preserve the established occurrence/attempt locking order and replace the simulated withdrawal-versus-parent-decision regression with coverage against the real approval/rejection service.
