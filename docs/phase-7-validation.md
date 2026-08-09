# Phase 7 validation

Phase 7 completes the trusted points workflow: parent decisions, append-only points, private child activity, auditable history, and household-local reports.

## Delivered guarantees

- Pending review includes the child, item snapshot, submission time, proposed points, occurrence version, and allowed parent actions.
- Approval, rejection, reversal, and positive correction require Parent Mode, CSRF protection, idempotency, and appropriate version checks.
- Approval writes exactly one award in the same transaction as the attempt decision, occurrence transition, audit event, and idempotency result.
- Reversal is terminal and writes an exact compensating ledger entry linked to the original award; no history is deleted or rewritten.
- Parent corrections are positive-only, require a reason, and display an explicit signed confirmation before submission.
- Children can see only their own balance and privacy-safe activity labels. Parent history exposes ordered attempts and traceable award/reversal/correction effects.
- Queue, ledger, and history use filter-bound keyset pagination; the UI exposes accessible incremental loading rather than truncating results.
- Daily, weekly, and monthly reports use household-local calendar boundaries and attribute occurrence awards/reversals to the occurrence date while attributing manual corrections to their local creation date.

## Verification evidence

- PostgreSQL migrations 1–12 applied successfully on a fresh database and the development stack.
- PostgreSQL tests cover same- and different-key races, approve/reject/withdraw/reverse competition, resubmission, archived children, five decision rollback stages, reversal/correction rollback stages, append-only and hostile-write constraints, tenant privacy, cursor binding, and ledger reconciliation.
- Reporting tests cover mixed occurrence states, rejection overlap, cancelled exclusion, late decision attribution, correction boundaries around Jakarta midnight, Berlin spring-gap/autumn-fold boundaries, and concurrent repeatable-read consistency.
- HTTP integration covers authentication, role authorization, resource hiding, CSRF no-write behavior, parent decisions, corrections, reports/history, and child-safe ledger projections.
- `go test ./...`, `go vet ./...`, and API/migration builds pass.
- Frontend formatting, ESLint, TypeScript, all 48 Vitest tests, and the Vite production build pass.
- Docker Compose rebuilt successfully; PostgreSQL, API, and frontend are healthy, migrations completed, the frontend is reachable, and `/health/ready` returned `{"status":"ready"}`.
- A strict independent product/security/accessibility audit approved the release after three remediation passes.

## Phase 8 handoff

The parent overview and report screens are already present. Phase 8 should treat them as integration scope, add the complete cross-role end-to-end journey, and finish the application-wide accessibility and security quality bar.
