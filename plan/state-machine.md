# Occurrence, Completion, and Points State Machine

## Statuses

- `not_started`: actionable by the assigned child.
- `pending_approval`: submitted by the child and awaiting a parent decision.
- `approved`: terminal success with one point award.
- `approval_reversed`: terminal state with an exact compensating ledger entry.
- `cancelled`: terminal state for a cancelled one-off occurrence.

Rejection and withdrawal finalize the current attempt but return the occurrence to `not_started`. Only `approved` appears in Child Today **Done**.

## Persistence contract

- `occurrences.current_status` is the canonical fast state and `occurrences.version` increments on every transition.
- `completion_attempts` is append-only; an occurrence may have multiple attempts after rejection or withdrawal.
- `point_ledger` is append-only with kinds `award`, `approval_reversal`, and `manual_correction`.
- `audit_events` is append-only and records actor, household, target, event, before/after status, safe metadata, idempotency key, and timestamp.
- State, attempt, ledger, and audit changes commit in one database transaction.

## Transition table

| Operation | Actor | From → To | Preconditions | Ledger effect | Audit event |
|---|---|---|---|---|---|
| Materialize | System | absent → `not_started` | Active source, scheduled local date, active child on date | None | `occurrence.created` |
| Submit | Assigned child | `not_started` → `pending_approval` | Same household/child, actionable date, no open attempt | None | `completion.submitted` |
| Withdraw | Same child | `pending_approval` → `not_started` | Open undecided attempt | None | `completion.withdrawn` |
| Approve | Parent | `pending_approval` → `approved` | Open attempt, positive point snapshot, no award | `award` for `+points_snapshot` | `completion.approved` |
| Reject | Parent | `pending_approval` → `not_started` | Open undecided attempt | None | `completion.rejected` |
| Reverse approval | Parent | `approved` → `approval_reversed` | Award exists, no prior reversal, reason supplied | Exact negative `approval_reversal` | `completion.approval_reversed` |
| Cancel task | Parent | `not_started` → `cancelled` | Cancellable one-off, reason supplied | None | `occurrence.cancelled` |
| Cancel pending task | Parent | `pending_approval` → `cancelled` | One-off task and open attempt | None | `occurrence.cancelled` |
| Manual correction | Parent | occurrence unchanged | Additive amount and reason supplied | Positive `manual_correction` | `points.corrected` |

Approved work must be reversed before any related administrative correction. Approved, reversed, and cancelled occurrences cannot be submitted again.

## Idempotency and concurrency

- Every mutation accepts an `Idempotency-Key`, scoped by household, actor/session, and route family.
- Store the request hash and finalized response. Replaying the same key and payload returns the original response; reusing a key with another payload is an error.
- Mutations lock the occurrence row and verify its version.
- Exact duplicate submit or approve calls return the existing successful result without new writes.
- Competing valid transitions use first-commit-wins; the loser receives `409 invalid_state_transition` with current status/version.
- Approval, award, and audit insertion occur atomically.

## Minimum database constraints

- Unique occurrence source/child/local-date identity.
- At most one open completion attempt per occurrence.
- At most one award per occurrence.
- At most one reversal per original award.
- Ledger amounts are non-zero; reversal exactly negates its referenced award.
- Completion terminal fields are mutually consistent.
- Occurrence version increases monotonically.
- Household consistency is enforced structurally where practical and always in service authorization.

## Forbidden actions

- A child cannot approve, reject, reverse, cancel, correct points, act for a sibling, or act across households.
- A parent cannot submit on a child's behalf in MVP.
- Pending work cannot be submitted twice.
- Approved work cannot be rejected, withdrawn, or cancelled.
- Reversed and cancelled occurrences cannot transition again.
- Definition edits never mutate dated snapshots, attempts, ledger entries, or audit events.
- No attempt, ledger entry, or audit event is deleted.

## HTTP behavior

- Use 404 where resource hiding prevents household or child enumeration.
- Use 403 for an authenticated actor lacking permission when revealing existence is safe.
- Use 409 with current state/version for stale valid-resource transitions.
- Return the stored 2xx result for exact idempotent replay.
- Audit and ledger writes are synchronous; failure rolls back the transition.

