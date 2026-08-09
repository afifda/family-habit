# Phase 6 Child Today and Completion Contract

Status: **implementation-ready review**  
Scope: PR-06 and the child-owned submit/withdraw portion of PR-07.

This contract refines the roadmap, approved state machine, Phase 5 scheduling contract, and initial OpenAPI draft. It does not implement parent decisions or points; those remain Phase 7.

## Release outcome

An active child session can open a household-local Today view, understand each item, submit eligible work once, and withdraw its own pending submission. These operations never write the point ledger. A parent may read a child's Today projection, but cannot submit or withdraw on the child's behalf.

## Blocking contract corrections

The following gaps in the initial OpenAPI/schema must be resolved during Phase 6:

1. A child must not use the optional `date` parameter to browse arbitrary historical or future behavioral data. For child sessions, omit `date` or require it to equal the server-derived household-local today; a different valid date returns `403 date_not_allowed`. Parent sessions may request an explicit date for support/testing, subject to the same materialization and projection rules.
2. Submit and withdraw must require an expected occurrence version using `If-Match` (or a request field with identical semantics). Idempotency prevents duplicate execution of one logical request; it does not prevent two different keys from racing on stale UI state.
3. `Occurrence` responses must expose `version`, presentation snapshots needed by detail (`description`, `icon`, `color`), and a child-safe `actionability`/available-actions projection. Clients must not infer permission solely from status.
4. Completion responses must distinguish attempt status from occurrence state and return the updated occurrence version. A withdrawn attempt is terminal `withdrawn` while its occurrence is again `not_started`.
5. The service needs a transactionally safe next-attempt allocation. `MAX(attempt_number)+1` without locking the occurrence is forbidden.

These are release blockers, not optional enhancements.

## Today date and materialization

The API derives `today` from the current UTC instant in the family's validated IANA timezone. Browser, API host, and PostgreSQL session dates are not authoritative.

`GET /api/v1/children/{childId}/today` performs the following in order:

1. Authenticate the current session and authorize the requested child.
2. Read and validate the family's timezone.
3. Derive or authorize the requested local date.
4. Invoke `EnsureDate(familyID, localDate)` before reading.
5. Return only the safe child projection scoped by both `family_id` and `child_id`.

Repeated and concurrent reads may materialize, but must return one canonical occurrence per source/date identity. A read never creates attempts, audit completion events, or ledger entries.

### Today membership

The response contains:

- all non-cancelled occurrences whose `local_date` equals the requested date;
- active one-off task occurrences with `due_date` before the requested date while their occurrence is `not_started` or `pending_approval`;
- no missed recurring occurrence from an earlier date;
- no cancelled or `approval_reversed` item;
- no sibling occurrence, definition, assignment, raw audit event, or parent-only decision metadata.

Items are deterministically ordered: group order `to_do`, `waiting_for_parent`, `done`; within a group, overdue tasks first, then habits before tasks, then normalized title and occurrence ID. Stable ordering prevents visual reordering across retries.

Group mapping is canonical:

| Occurrence state | Group | Child action |
|---|---|---|
| `not_started` and actionable | `to_do` | `submit` |
| `pending_approval` | `waiting_for_parent` | `withdraw` |
| `approved` | `done` | none |
| `approval_reversed` or `cancelled` | omitted | none |

An occurrence included for the selected date but no longer actionable may be shown without a mutation action. The API supplies `availableActions`; the UI does not recreate business rules.

### Actionability

- A recurring habit can be submitted only when its `local_date` equals the server-derived household-local current date.
- An active one-off task can be submitted when its `due_date` is on or before current local date.
- Future occurrences cannot be submitted, including through a parent-authorized dated read.
- An archived child cannot read Today or mutate completions. Authentication should already demote an archived active-child session; the endpoint also enforces this at transaction time.
- `overdue` is neutral derived presentation, never a lifecycle state. Child copy includes the explicit due date and avoids punishment language.

## Safe Today representation

The response includes `childId`, selected `date`, `timezone`, groups/counts, and occurrence cards. Each card includes:

- occurrence ID and version;
- `habit` or `task` type;
- immutable title, description, icon, color, and points snapshots;
- local date and nullable due date;
- lifecycle status, group, and due state;
- current pending completion ID only when the requesting child may withdraw it;
- explicit available actions.

Do not expose assignment IDs, task management state/reason, family member IDs, audit metadata, decision-maker identity, sibling data, internal idempotency data, or database timestamps not needed by the child journey.

## Submit transition

`POST /api/v1/occurrences/{occurrenceId}/completions` requires a child session, CSRF token, `Idempotency-Key`, and expected occurrence version.

Within one transaction the service:

1. reserves or replays the session-scoped idempotency record with a canonical request hash;
2. selects the occurrence by `family_id`, session `active_child_id`, and occurrence ID `FOR UPDATE`;
3. rechecks the child is active, source is active/actionable, expected version matches, state is `not_started`, and no open attempt exists;
4. allocates the next positive attempt number while holding the occurrence lock;
5. inserts one pending `completion_attempt`;
6. changes the occurrence to `pending_approval`, increments its version once, and updates its timestamp;
7. inserts allowlisted `completion.submitted` audit data with the child actor, session, status change, occurrence subject, and idempotency key;
8. finalizes the stored response and commits.

It writes no ledger row. The successful response is `201` and contains the pending attempt plus occurrence status/version.

## Withdraw transition

`DELETE /api/v1/completions/{completionId}` requires the same child session controls, `Idempotency-Key`, and expected occurrence version.

Within one transaction the service locks the family/child-scoped occurrence and its current pending attempt, verifies the completion ID names that open attempt, checks expected version and `pending_approval`, then:

- marks the attempt `withdrawn` with `decided_at` and no parent decision-maker;
- changes the occurrence to `not_started` and increments its version once;
- inserts `completion.withdrawn` audit data;
- stores the finalized response and commits.

It writes no ledger row. A later submit creates a new attempt with the next attempt number; it never reopens or overwrites the withdrawn row.

## Idempotency, concurrency, and errors

- Idempotency scope is `(family, session, route family, key)`. Keys are 1–128 characters and retained for the documented server window.
- Exact replay with the same canonical request returns the original status/body without a new attempt, state/version change, audit event, or timestamp change.
- Reuse of a key with a different occurrence, completion, version, or payload returns `409 idempotency_conflict`.
- Competing requests with different keys are serialized by the occurrence row lock. First commit wins; the loser receives `409 invalid_state_transition` or `409 version_conflict` with safe current status/version.
- Database uniqueness for one open attempt is a final defense, not the primary control path.
- Missing/invalid CSRF or authentication returns `401`/`403` according to the established API convention before disclosing a resource.
- Nonexistent, sibling, and cross-family occurrence/completion IDs return the same resource-hiding `404`.
- Parent/shared sessions receive `403 child_mode_required`; a parent session never becomes a child actor implicitly.
- Malformed UUID/date/header input returns the shared validation envelope; internal database details are never returned.

An idempotency reservation and business transaction must not leave a permanently in-progress record after rollback. Concurrent same-key requests must wait/replay or return the documented retryable conflict; they must never both execute.

## Audit and data safety

Submit and withdraw state, attempt, occurrence version, audit event, and finalized idempotency response commit or roll back together. Audit metadata is an allowlist limited to safe identifiers and attempt number. It excludes nickname, description text, cookies, CSRF/session tokens, PIN/password data, request headers, unrestricted bodies, and network fingerprints.

Completion attempts and audit events are append-only from normal application repositories. Phase 6 must not add update/delete paths except the narrowly constrained pending-attempt finalization performed by the transition service. Occurrence snapshots cannot be edited by completion operations.

## Child interface acceptance

- The default child route loads Today for the active profile and displays that profile unambiguously.
- Sections are labelled exactly **To do**, **Waiting for parent**, and **Done**, including counts.
- Cards show title, points with text, item type, and a meaningful due label; icons/colors are supplementary.
- Detail exposes the immutable description and due information and retains the same primary action.
- The primary submit label is **I did it**. Pending work offers **Withdraw submission**; approved work has no action.
- One activation disables the action immediately until the request resolves. Exact retry reuses the same idempotency key; a new logical action gets a new key and latest version.
- Success and failure are announced through a polite accessible live region, and focus moves predictably when an item changes group.
- Loading skeleton/status, true empty state, recoverable server error, offline/retry copy, permission loss, and stale/conflict refresh states are implemented.
- Retry never fabricates success. On an ambiguous network failure it replays the same key before fetching current state.
- Controls are keyboard-operable, have visible focus, meaningful accessible names, and practical 48-by-48 CSS-pixel targets. Status is never conveyed only by color or motion; reduced motion and 200% text zoom remain usable.

## Required test matrix

### API and service behavior

- Today calls `EnsureDate` and returns daily/selected-weekday occurrences exactly once.
- Household-local today is correct for a Jakarta UTC date boundary and on both Berlin DST gap/fold sides.
- Today includes actionable overdue tasks, excludes past recurring habits, cancelled/reversed items, and future items.
- Exact response grouping, due state, snapshots, available actions, and stable ordering are asserted.
- Child omitted date succeeds; same-date succeeds if supported; past/future date is forbidden. Parent explicit date succeeds.
- Submit changes `not_started` to `pending_approval`, creates attempt 1, increments version once, writes one audit event, and writes zero ledger entries.
- Withdraw finalizes attempt 1, returns occurrence to `not_started`, increments version once, writes one audit event, and writes zero ledger entries.
- Resubmit after withdrawal creates attempt 2 and preserves attempt 1.
- Recurring current-date and overdue task submission succeed; future task and past recurring submission fail.
- Transaction fault injection at attempt, occurrence, audit, and idempotency-finalization stages proves total rollback.

### Idempotency and concurrency

- Sequential exact submit replay returns byte-equivalent stored success with one attempt/audit/version increment.
- Sequential exact withdraw replay returns stored success with one withdrawal/audit/version increment.
- Same key with changed target or version conflicts.
- Two concurrent submits with the same key execute once and both resolve consistently.
- Two concurrent submits with different keys yield one success, one state/version conflict, one open attempt.
- Concurrent submit versus task cancellation yields one legal terminal result with consistent attempt state and audit history.
- Concurrent withdraw versus Phase 7-style parent decision is covered at the service boundary now or made a mandatory Phase 7 regression: only one transition wins.
- Concurrent withdraw requests cannot both finalize or increment twice.

### Authorization and privacy

- Unauthenticated, expired, and revoked sessions fail.
- Shared and parent sessions cannot submit/withdraw; parent read access remains read-only.
- Child A cannot read Child B Today or act on Child B occurrence/completion.
- A valid cross-family child, occurrence, and completion UUID is hidden exactly like a nonexistent UUID.
- Archived-child session is demoted/denied and cannot race archival to commit a submission.
- CSRF failure and missing/oversized idempotency key cause no writes.
- Responses and logs are checked for sibling fields, secrets, raw database errors, and forbidden audit metadata.

### Database invariants

- At most one pending attempt exists under concurrent direct/service writes.
- Attempt numbers are unique, positive, monotonic per occurrence.
- Attempt decision shape is valid for pending and withdrawn rows.
- Occurrence status/version, attempt, audit, and idempotency response stay consistent after rollback/retry.
- Normal application repositories cannot delete attempts/audit rows or mutate snapshots.

### Frontend and accessibility

- Component tests cover every group and item type, empty/loading/error/offline/permission/conflict states, detail open/close, submit, withdraw, ambiguous retry, and duplicate-tap suppression.
- Tests assert focus restoration/movement, live-region announcements, keyboard operation, accessible names, section headings/counts, non-color status text, and disabled/busy semantics.
- API mocks prove an ambiguous retry reuses a key and a later resubmit uses a new key/latest version.
- Responsive checks cover narrow phone, tablet, desktop, 200% zoom, long titles/descriptions, and reduced motion.
- Production format, lint, TypeScript, unit/component, accessibility scan, and build commands pass.

## Phase 6 release gate

Phase 6 is complete only when:

- the blocking OpenAPI corrections above match the implementation;
- migrations apply from an empty PostgreSQL 16 database and database invariants pass;
- all required state, rollback, concurrency, tenant-isolation, timezone, and accessibility tests pass;
- live child-session submit, ambiguous replay, withdraw, and resubmit flows pass;
- point ledger row count and balance remain unchanged throughout every Phase 6 flow;
- child attempts to read or act for siblings/other households all fail without enumeration;
- backend format, test, vet, race-relevant integration, and build checks pass;
- frontend format, lint, typecheck, tests, accessibility checks, and production build pass;
- the running PostgreSQL, API, and frontend remain healthy.

The roadmap gate must remain unchecked until validation evidence demonstrates every condition above.
