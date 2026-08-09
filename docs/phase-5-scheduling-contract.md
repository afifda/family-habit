# Phase 5 Scheduling Contract

Status: **accepted implementation contract**  
Scope: recurring habits, child assignments, schedules, dated occurrences, and one-off tasks.

This contract refines PR-04 and PR-05 without changing the approved occurrence state machine. Where an earlier schema or API draft is less precise, this document defines the Phase 5 implementation and release behavior.

## Core model

- A habit is the stable household-owned identity of recurring work.
- Habit presentation is effective-dated and immutable by revision. A revision contains the title, optional description, icon, and color plus an inclusive `effective_from` and optional inclusive `effective_until`.
- An assignment is one effective-dated version of a habit assigned to one child. It contains the points and schedule that apply during its inclusive date range.
- Multi-child habits use one distinct assignment row per child. They never share an occurrence.
- A schedule is either every day or a non-empty set of explicit weekdays. Weekdays are evaluated against household-local calendar dates and do not depend on the configured calendar week start.
- A one-off task belongs to exactly one child and has one due date and one occurrence.
- An occurrence is the dated, child-specific record presented to completion and reporting workflows. It snapshots the effective presentation and points.

All household-owned reads and writes require `family_id`. Cross-household relationships are rejected structurally where possible and by service authorization in every case.

## Effective dates and revisions

An edit effective on local date D means **this date and future dates**:

1. The prior revision or assignment version ends on D minus one calendar day.
2. A replacement revision or assignment version begins on D.
3. Past occurrences are not changed.
4. Pending, approved, approval-reversed, or cancelled occurrences are not changed.
5. An occurrence with any completion attempt is historical evidence and is not changed, even if its current state has returned to `not_started` after rejection or withdrawal.
6. A future `not_started` occurrence with no attempts may be deleted and deterministically materialized again from the replacement definition. Snapshot columns are never updated in place.

Effective ranges for the same habit and child must not overlap. This invariant is enforced by PostgreSQL in a way that remains safe under concurrent transactions. Service-level pre-checks exist for useful validation messages but are not the concurrency boundary.

A presentation edit applies to every assignment whose active date range intersects the new revision. A points or schedule edit creates replacement assignment versions only for the selected children. The parent interface must preview the affected children and require an explicit effective date.

Deactivation effective on D makes D the first inactive date. It closes applicable ranges on D minus one day and removes only materialized occurrences on or after D that are `not_started` and have no attempts. It preserves every other occurrence and all history.

## Atomic multi-child assignment

Creating a recurring habit assignment for multiple children is one atomic operation:

- The request contains a non-empty, duplicate-free set of active child IDs.
- Every child must belong to the authenticated household and be active at commit time.
- One independent assignment is created for each child with the same requested points, schedule, and start date.
- Either all requested assignments commit or none commit.
- The response identifies every created child-specific assignment.
- A replay with the same idempotency key and payload returns the original successful response without adding assignments.
- A reused key with a different payload returns an idempotency conflict.

Concurrent child archival, overlapping assignment creation, or habit deactivation cannot produce a partially valid result. Transactions lock or otherwise recheck the relevant rows before commit, while database constraints provide the final overlap and tenant-integrity guarantees.

## Deterministic lazy occurrence generation

Recurring occurrences are generated lazily. The scheduling service exposes an operation equivalent to `EnsureDate(family, localDate)`; Phase 6 invokes it when serving the household-local Today view.

The generator:

1. Receives an explicit household and local calendar date. It does not infer a date from the database server or container timezone.
2. Selects active children and habit revisions, assignments, and schedules whose inclusive ranges contain that date.
3. Evaluates the explicit weekday of the local date.
4. Builds one candidate per assignment and child.
5. Inserts the occurrence using its deterministic source, child, and local-date identity.
6. Treats a uniqueness conflict as another transaction having created the canonical row, then reads and returns that row.

There is no rolling pre-generation horizon in the MVP. Repeated and concurrent calls for the same family and date produce the same set of rows and never duplicate occurrences.

Each occurrence snapshots at least:

- title;
- description, icon, and color used by child detail presentation;
- points;
- item type;
- local date and, for a task, due date.

Snapshots are immutable after insertion. The only replacement permitted is deletion and deterministic rematerialization of an unstarted occurrence that has no attempts, under the effective-date rules above.

Recurring occurrences are actionable only when their `local_date` equals the household-local current date. A missed recurring habit does not become overdue and is not carried into later Today views.

## One-off tasks

Creating a task and its sole occurrence is one transaction. The occurrence uses the task due date as both its local occurrence date and due date, and its unique task identity prevents a second occurrence.

A task may be edited only while its occurrence is `not_started` and has no completion attempts. An edit may change its title, description, presentation, points, or due date, but not its child. The service replaces the unstarted occurrence in the same transaction so the task and occurrence cannot disagree.

A task may be cancelled from either `not_started` or `pending_approval`:

- A non-blank, child-safe reason is required.
- Cancellation changes the occurrence to terminal `cancelled` and increments its version.
- If an attempt is pending, it is finalized as cancelled in the same transaction.
- The task state, occurrence, attempt when present, and audit event commit atomically.
- Approved, approval-reversed, or already-cancelled tasks cannot be cancelled. A competing transition receives `409 invalid_state_transition` with the canonical status and version.

An active, incomplete task remains actionable on and after its due date until it is completed or cancelled. `overdue` is a derived due-state where `due_date` is before household-local today; it is not stored as an occurrence lifecycle status. Child-facing language is neutral and includes the explicit due date.

## History and audit rules

- Definitions and effective ranges may be closed but are not hard-deleted after they have produced history.
- Occurrences with attempts are never deleted.
- Dated snapshots, completion attempts, ledger entries, and audit events are never rewritten to reflect later definition edits.
- Habit and assignment changes, task edits, deactivations, and cancellations produce safe audit events with actor, household, target, effective date or before/after state, and idempotency correlation where applicable.
- Audit metadata uses an allowlist and never includes passwords, PINs, cookies, tokens, CSRF secrets, or unrestricted request bodies.

## Authorization, idempotency, and concurrency

All Phase 5 management operations require:

- an authenticated, currently unlocked Parent Mode session;
- CSRF validation for state-changing requests;
- server-side membership and household scoping;
- resource-hiding `404` behavior for nonexistent and cross-household identifiers;
- rejection of archived children and inactive source definitions where applicable.

Child and shared sessions cannot list habit definitions or assignments and cannot create, edit, deactivate, or cancel Phase 5 resources.

Every mutation accepts an `Idempotency-Key`, scoped by household, session, and route family. The server stores a canonical request hash and finalized response. Exact replay returns the stored response; key reuse with another payload returns a conflict. Validation failures that did not start a durable operation do not masquerade as a successful replay.

Definition and task mutations use an expected version or equivalent conditional-write contract. Transactions lock affected rows, validate the current version and state, and use first-commit-wins. A stale competing mutation returns `409` with the current version rather than silently overwriting the winner.

Database uniqueness and exclusion constraints, not process-local locks, are the final defense against duplicate occurrences and overlapping ranges. This remains correct with multiple API processes.

## Household time and DST

Dates are PostgreSQL `date` values interpreted in the household's validated IANA timezone. Timestamps remain UTC instants. The operating-system, browser, API container, and PostgreSQL session timezone are not scheduling authorities.

Changing the household timezone affects later current-date derivation and future materialization. It does not rewrite occurrence local dates, snapshots, task due dates, attempts, or history already stored.

The minimum scheduling test matrix covers:

- daily schedules across all weekdays;
- every individual selected weekday and multi-day combinations;
- inclusive effective start and end boundaries, including D minus one and D;
- Sunday represented consistently with the implementation's canonical weekday mapping;
- a UTC instant that falls on the next local date in `Asia/Jakarta`;
- `Europe/Berlin` instants on both sides of the spring DST gap;
- both UTC instants represented by the autumn DST fold;
- proof that gap and fold instants derive the correct local calendar date exactly once;
- timezone changes preserving existing dated rows while later generation uses the new timezone;
- repeated and concurrent generation for the same date;
- different children assigned the same habit producing independent occurrences.

Because recurrence is calendar-date based and has no wall-clock execution time, a DST gap or fold never creates zero or two occurrences for one assignment and local date.

## Parent experience

The Parent Mode screen separates active recurring work, active one-off tasks, and inactive or cancelled history. It implements accessible loading, empty, error, retry, and success states.

The progressive create/edit flow includes:

- an initial recurring versus one-off choice;
- active-child selection, with multiple children only for recurring habits;
- title and optional description/icon/color;
- points from 1 through 10,000, defaulting to 5 with quick choices 1, 2, 5, and 10;
- every-day or selected-weekday scheduling and an effective start date for recurring work;
- one child and a due date for one-off work;
- a review summary before saving;
- explicit **This and future** wording and an effective date for recurring edits;
- confirmation that past, waiting, and completed history will not change;
- confirmation and a required reason for cancellation.

Forms provide inline errors and a focusable error summary. Weekday controls expose their names and selected states to assistive technology. Dialogs trap and restore focus appropriately, keyboard operation is complete, and pending mutations prevent accidental duplicate submission.

## Phase 5 release gate

Phase 5 is complete only when all of the following pass:

- migrations apply from an empty PostgreSQL 16 database;
- effective ranges cannot overlap under sequential or concurrent writes;
- effective edits and deactivation preserve every protected occurrence and replace only eligible future unstarted occurrences;
- title, presentation, type, and point snapshots remain unchanged by later definition edits;
- deterministic generation passes weekday, boundary, timezone, DST, retry, and concurrent-insert tests;
- one habit assigned to multiple children produces independent assignments and occurrences atomically;
- task create, allowed edit, overdue, pending cancellation, and forbidden-state cases pass;
- exact idempotent replay and mismatched-payload conflict behavior pass for every mutation family;
- parent, child, shared, unauthenticated, archived-child, wrong-child, and cross-household authorization cases pass;
- backend unit, PostgreSQL integration, race/concurrency, vet, and build checks pass;
- frontend formatting, lint, type checking, component, interaction, accessibility, and production-build checks pass;
- the OpenAPI contract matches implemented inputs, responses, errors, idempotency, and concurrency semantics;
- live parent flows for recurring multi-child work and one-off task creation/edit/cancellation pass without regressions to Phases 1–4;
- the running API, frontend, and PostgreSQL services remain healthy.

The roadmap gate remains incomplete until the implementation, tests, contract, and validation evidence satisfy every item above.
