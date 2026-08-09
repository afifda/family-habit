# Phase 7 Parent Approval, Points, and Reporting Contract

Status: **implementation-ready review**  
Scope: PR-07, PR-08, and the Phase 7 reporting foundation of PR-09.

This contract refines the approved occurrence state machine, Phase 0 decisions, Phase 6 completion contract, current PostgreSQL schema, and draft OpenAPI. Phase 7 delivers the trusted parent decision and points workflow plus the data APIs and core screens needed to understand it. The broader parent dashboard and polished report views remain Phase 8.

## Release outcome

A parent can review each pending child submission, approve or reject it exactly once, reverse an erroneous approval without erasing history, and add a positive reasoned correction. A child can see only their own balance and recent point activity. A parent can see at least 30 days of each child's occurrence history and household-local daily, weekly, and monthly aggregates. Every balance delta is an immutable ledger row and every mutation is authorized, version-checked, idempotent, transactional, and audited.

## Blocking contract corrections

The following drift in the draft OpenAPI/schema must be corrected before Phase 7 can pass:

1. `approve`, `reject`, and `reverse` must require `If-Match` with the expected positive occurrence version. Idempotency alone does not prevent distinct keys acting on stale parent UI. Responses expose the incremented version.
2. The pending-review representation cannot reuse the child-oriented `Completion` alone. It must include the child nickname/avatar/color and immutable occurrence title, type, points, local/due date, submission time, attempt number, occurrence status/version, and explicit parent actions. It must not expose secrets or mutable definitions.
3. Public ledger kinds must use the canonical persistence names `award`, `approval_reversal`, and `manual_correction`, or the API must document a deliberate stable mapping. The current OpenAPI names `approval_award` and `parent_correction` drift from the database and state machine.
4. Historical occurrence responses cannot require a Child Today `group`. Reversed and cancelled occurrences are deliberately omitted from Today and have no valid Today group. History needs its own projection with decision summary and no child actionability.
5. Decision responses must expose enough linkage to reconcile the transaction: completion/attempt ID and status, occurrence ID/status/version, and, for approval/reversal, the created ledger entry or its identifier and signed amount.
6. Report counting and point attribution must follow the definitions below. The initial schema leaves `submitted`, `rejected`, `incomplete`, and `pointsEarned` ambiguous and could produce overlapping totals that appear contradictory.
7. Ledger structural protection must include one award per occurrence, one reversal per award, exact signed compensation, and household/child/occurrence consistency. Existing reversal checks are useful but the one-award invariant and all scoped links must be verified by migration/integration tests.
8. Pending work for an archived child remains parent-reviewable. Parent reads and decisions must not depend on the child being active; child mutations remain forbidden after archival.

## Canonical transitions

All mutations require an authenticated, currently unlocked parent session, valid CSRF token, an opaque 1–128 character `Idempotency-Key`, and normal tenant-scoped resource hiding. Occurrence mutations additionally require `If-Match`. The service preserves the Phase 6 lock order: reserve idempotency, lock the tenant-scoped occurrence, then lock/read its current attempt and related ledger rows.

| Operation | Preconditions | Atomic changes | Result |
|---|---|---|---|
| Approve | Occurrence is `pending_approval`; named completion is its sole open `pending` attempt; expected version matches; positive snapshot is 1–10,000; no award exists | Attempt becomes `approved` with parent/time; occurrence becomes `approved` and version increments once; insert one `award` for `+points_snapshot`; insert `completion.approved` audit; finalize idempotency response | Terminal approved work and one traceable award |
| Reject | Same pending/open-attempt/version checks; optional trimmed child-safe reason, maximum 500 characters | Attempt becomes `rejected` with parent/time/reason; occurrence becomes `not_started` and version increments once; insert audit; finalize response | Child may create a new numbered attempt; no ledger write |
| Reverse | Occurrence is `approved`; named completion is its approved attempt; expected version matches; exactly one matching award exists; no reversal exists; required trimmed reason is 1–500 characters | Occurrence becomes `approval_reversed` and version increments once; insert `approval_reversal` exactly negating original award and referencing it; insert audit; finalize response | Terminal reversed work; original attempt and award remain immutable |
| Manual correction | Tenant-scoped child exists, including archived child; positive integer amount is 1–10,000; required trimmed reason is 1–500 characters | Insert `manual_correction` with parent actor; insert `points.corrected` audit; finalize response | Balance increases; no occurrence changes |

Reject never reopens or edits the rejected attempt. A later child submission creates the next positive, monotonically increasing attempt number. Reversal does not change the approved attempt to another attempt status; the occurrence and compensating ledger record express the administrative reversal. Approved, reversed, and cancelled occurrences cannot otherwise transition. An approved occurrence must be reversed, not manually corrected downward.

The optional rejection reason is returned to the assigned child in history using child-safe text. The UI labels this as guidance, not failure or punishment. Reversal and manual-correction reasons are parent administrative records; child ledger activity may use neutral derived copy and must not expose parent identity or internal audit metadata.

## Ledger invariants and balance

`point_ledger` is the sole authority for balances. No mutable balance column, occurrence-state sum, or completion count may be treated as authoritative.

- Balance is `COALESCE(SUM(amount), 0)` for exactly `(family_id, child_id)` across all ledger history. It may not become negative under MVP operations because corrections are positive and a reversal can only negate an existing award once.
- `award`: positive amount equal to the immutable occurrence `points_snapshot`; references the exact approved completion attempt and occurrence; at most one per occurrence and per approved attempt.
- `approval_reversal`: negative amount exactly equal to `-original.amount`; references the same family, child, and occurrence plus its original award; at most one per original award.
- `manual_correction`: positive amount from 1–10,000; has no occurrence, completion, or reversal reference; has a nonblank reason and parent actor.
- Ledger rows are append-only through normal application repositories. No update or delete route exists. Corrections and reversals never rewrite prior rows.
- Every reference is tenant-consistent. Cross-family or cross-child occurrence, attempt, award, actor, and reversal combinations are rejected structurally where practical and always transactionally.
- Ledger creation and its corresponding audit event commit or roll back together. Approval additionally couples attempt and occurrence state; reversal couples occurrence state.
- The ledger list uses deterministic keyset ordering `(created_at DESC, id DESC)`. Cursors are opaque, filter-bound, and cannot move a caller to another child or household.
- Exact idempotent replay returns the originally stored HTTP status/body and produces no new timestamp, ledger row, audit event, decision, or version increment.

The child-safe ledger entry contains ID, canonical kind, signed amount, neutral display label, occurrence ID/title snapshot when applicable, and UTC creation time. Parent responses may also include the reason and original-entry linkage. Neither projection exposes user IDs, session IDs, sibling IDs, unrestricted audit metadata, or mutable task/habit data.

## Pending queue and parent decisions

`GET /api/v1/review/pending` is parent-only and reads all pending attempts in the current household, including those belonging to archived children. Optional `childId` is authorized within the household before filtering. Results use deterministic keyset ordering `(submitted_at ASC, completion_id ASC)` so the oldest submission is reviewed first.

Each item contains:

- completion ID, attempt number, and submission UTC timestamp;
- occurrence ID, expected version, `pending_approval` status, immutable title/type/points snapshots, local date, and nullable due date;
- child ID plus nickname/avatar/color presentation fields;
- explicit `availableActions: [approve, reject]`.

The queue returns no PIN verifier, child session state, definition/assignment IDs, password or CSRF data, audit metadata, or data from another family. A decided or withdrawn attempt cannot remain in a fresh queue response.

Approval and rejection target the completion ID but lock and validate its occurrence. Nonexistent, cross-family, or mismatched completion/occurrence combinations return the same `404`. A stale valid household item returns `409 version_conflict` with a safe current status/version so the UI refreshes. A legal competing transition that already won returns `409 invalid_state_transition`. Exact replay is the only case in which the stored success is returned after state has advanced.

## Read models

### Balance and recent activity

`GET /api/v1/children/{childId}/points` returns the ledger-derived balance and an `asOf` UTC timestamp. Parent sessions may read any child in their household; a child session may read only its own active-child ID. Archived children remain parent-readable.

`GET /api/v1/children/{childId}/points/ledger` returns a keyset page. Default 25, maximum 100. Same-child responses use the safe projection described above; parent responses include administrative reason/linkage. The Phase 7 child activity screen shows the balance and recent entries without sibling comparisons, ranks, punishment language, or a correction/reversal action.

### Occurrence history

`GET /api/v1/children/{childId}/occurrences` is parent-only in Phase 7. It supports inclusive household-local `from` and `to` dates, defaults to `[today - 29 days, today]`, and rejects reversed ranges or a range greater than 366 days. Pagination is deterministic `(local_date DESC, id DESC)` and bound to the date filters.

History entries contain immutable occurrence presentation and points snapshots, local/due date, current status/version, and ordered attempt summaries with submitted/decided UTC timestamps, attempt status, and child-safe decision reason. Approved/reversed entries include their award/reversal deltas; cancelled entries remain visible. They contain no Today group/action, raw definition, session, actor user ID, or unrestricted audit payload.

### Reporting periods and timezone semantics

`GET /api/v1/reports/children/{childId}?period=day|week|month&anchorDate=YYYY-MM-DD` is parent-only. `anchorDate` is a calendar date in the family's current validated IANA timezone, not the browser or database timezone.

- Day: `startDate = endDate = anchorDate`.
- Week: start is the nearest preceding-or-equal configured `week_start` (`sunday` or `monday`); end is start plus six local calendar days.
- Month: start is the first and end is the last calendar date of the anchor's month.
- Boundaries are inclusive PostgreSQL dates. No UTC-duration arithmetic defines them, so 23/25-hour DST days remain one report day.
- A later household timezone/week-start change affects how a newly requested period is selected, but never rewrites stored occurrence `local_date`. The response always returns timezone, week start, and explicit start/end dates used.

Occurrence metrics are scoped by the occurrence's immutable `local_date` within the period and are mutually exclusive current-state counts:

- `assigned`: all non-cancelled occurrences in the period; it equals `pending + approved + reversed + incomplete`.
- `pending`: current status `pending_approval`.
- `approved`: current status `approved`.
- `reversed`: current status `approval_reversed`.
- `incomplete`: current status `not_started`.
- `submitted`: distinct non-cancelled occurrences in the period with at least one completion attempt; this is an activity measure and may overlap the state counts.
- `rejected`: distinct non-cancelled occurrences in the period with at least one rejected attempt; this is an attempt-history measure and may overlap `submitted` and the current-state counts.
- `cancelled` is reported separately and excluded from `assigned`; it is never silently counted as incomplete.

`pointsEarned` is the net ledger delta attributable to occurrences whose immutable `local_date` is in the period: awards plus their reversals, regardless of the later UTC decision timestamp. `manualCorrections` is a separate positive total bucketed by the correction `created_at` converted to the family's timezone. `netPointsChange = pointsEarned + manualCorrections`. This separation preserves occurrence-performance meaning while reconciling ledger activity. Reports do not materialize missing historical occurrences; they aggregate durable rows only. Calling a report has no writes.

## Idempotency, concurrency, and rollback

- Idempotency scope remains `(family, session, route family, key)` with a canonical request hash that includes target, expected version, and normalized body. Changed target/version/reason/amount with the same key returns `409 idempotency_conflict`.
- Same-key concurrent calls wait and replay one finalized result, or return the documented retryable in-progress conflict; they never both execute.
- Different-key decisions serialize on the occurrence. Approve versus reject, approve versus withdraw, reject versus withdraw, and duplicate approvals have one legal winner.
- Approval's attempt decision, occurrence transition/version, award, audit event, and finalized idempotency response are one transaction. Equivalent atomicity applies to rejection, reversal, and correction.
- A transaction failure cannot leave a reserved key permanently executing. Retrying after rollback can safely execute once.
- Read endpoints use a transactionally consistent snapshot where a split read could otherwise return a new balance with a missing entry or a decision without its ledger row.

## Authorization, privacy, and audit

- Queue, decisions, reversal, correction, parent history, and reports require unlocked Parent Mode and server-side owner authorization. UI visibility is not authorization.
- A child can read only their own balance and safe ledger activity. Siblings cannot access each other's balance, ledger, history, reports, queue, or decisions. Parent/shared sessions cannot be silently treated as child sessions.
- Cross-household valid UUIDs are indistinguishable from nonexistent UUIDs. Filter parameters and cursors receive the same tenant checks as path resources.
- Archived child submissions remain reviewable by the parent, and archived child history/ledger remain readable. Archived child sessions cannot read or mutate.
- Audit events are append-only and record the parent actor, session, target, before/after status, idempotency key, and an allowlist of safe identifiers/amounts/reason where required. They exclude nickname, free-form occurrence descriptions, secrets, cookies, headers, raw request bodies, and network fingerprints.
- Logs and errors contain request IDs and safe error codes, never raw database details, PIN/password material, session/CSRF values, idempotency payloads, or another household's data.
- Reasons are trimmed, length-limited plain text. They are rendered as text, never HTML. Rejection reason must be child-safe; reversal/correction reason is parent-only administrative text by default.

## UX acceptance

### Parent approval queue

- The page identifies Parent Mode and presents oldest submissions first with child identity, item title/type, proposed points, submitted time, and local due context.
- Each item has explicit **Approve** and **Send back** controls; rejection is not labelled failure. Optional guidance is clearly described as visible to the child.
- Approval shows the exact points to be awarded. Reversal and positive correction require a confirmation that names the signed balance effect; routine approval/rejection does not add unnecessary modal friction.
- One activation immediately disables both decision controls for that item and exposes busy semantics. An ambiguous failure replays the same key; a new decision uses a new key and latest version.
- Success removes/finalizes the item, updates pending count and affected cached balance/history, announces the result through a polite live region, and moves focus predictably to the next item or queue heading.
- Stale/conflict responses explain that the item changed and refresh it without fabricating success. Loading, empty, recoverable server error, offline/retry, locked Parent Mode, and permission-loss states are implemented.

### Child points and parent history

- The child view leads with their own current balance and recent activity in warm, neutral language. Signed negative reversal rows are described as an approval correction, not punishment or lost points caused by the child.
- The parent history view defaults to the latest 30 household-local days and makes attempts, decisions, award, reversal, and correction effects traceable without displaying raw audit data.
- No view ranks siblings, surfaces a leaderboard, uses punitive red/failure copy, or conveys status only through color/icon/motion.
- Controls are keyboard-operable with visible focus and practical 48-by-48 CSS-pixel targets. Headings, lists/tables, dialog names, busy/disabled state, validation, and live feedback are programmatically available. Core screens remain usable at 200% text zoom and with reduced motion.

## Required release test matrix

### State transitions and ledger

- Approve finalizes the pending attempt, changes the occurrence, increments version once, awards the exact snapshot once, audits once, and changes balance by that amount.
- Reject with no reason and with valid child-safe reason finalizes the attempt, returns to `not_started`, increments once, creates no ledger row, and permits a higher-numbered resubmission.
- Reverse requires a reason, transitions approved to terminal reversed, increments once, creates one exact compensation linked to the award, preserves the approved attempt/award, and restores the prior balance.
- Manual correction accepts bounds 1 and 10,000, rejects zero/negative/overflow/blank or overlong reason, changes no occurrence, and audits exactly once.
- Approved cannot be rejected/withdrawn/cancelled; reversed cannot be submitted/approved/reversed again; correction cannot masquerade as negative punishment.
- Archived-child pending approval and historical balance remain parent-operable/readable while child access remains denied.

### Idempotency, concurrency, and rollback

- Sequential and concurrent exact replay of approve, reject, reverse, and correction returns the stored result with one transition/ledger/audit/version effect.
- Same key with changed target, version, reason, or amount conflicts with no writes.
- Concurrent different-key approve/approve produces one award; approve/reject, approve/withdraw, reject/withdraw, reverse/reverse, and reverse against stale reads each produce one legal winner and coherent loser response.
- Approval racing child archive still resolves legally: an already-pending item remains reviewable and no child mutation escapes archival.
- Fault injection after every write stage proves complete rollback, including no orphan ledger/audit/idempotency record and safe retry.
- Database tests attempt duplicate awards, duplicate reversals, mismatched family/child/occurrence/attempt references, wrong-sign amounts, inexact compensation, mutation/deletion, and invalid kind shapes.

### Reads, periods, and reconciliation

- Zero-entry balance is zero; balance equals the signed ledger sum after mixed awards, reversals, and corrections.
- Ledger/history pagination has no duplicates or omissions when timestamps tie and rejects cursor/filter/child reuse.
- Default history is exactly 30 inclusive local dates and includes approved, reversed, rejected/resubmitted, pending, incomplete, and cancelled history with correct snapshots.
- Day/week/month boundaries pass for Sunday and Monday week starts, month/year/leap-day edges, Jakarta UTC boundary, and both sides of Berlin DST gap/fold.
- Metrics follow the explicit overlap/equality rules for never-submitted, pending, rejected-then-resubmitted, approved, reversed, and cancelled occurrences.
- Points attribution verifies an occurrence approved or reversed after its local-date period, plus corrections near timezone midnight. `pointsEarned + manualCorrections = netPointsChange` for the defined period buckets.
- Report/history reads create no occurrences, attempts, ledger rows, audits, or idempotency records.
- Read consistency tests never observe an award without its decision or a balance without the corresponding ledger entry.

### Authorization and privacy

- Unauthenticated, expired/revoked, locked-parent, shared, child, and wrong-mode sessions fail every parent-only operation appropriately; CSRF failures cause no writes.
- Child A can read only Child A balance/safe ledger and cannot enumerate Child B or any report/history/queue/decision route.
- Valid cross-family child, completion, occurrence, ledger cursor, and filter UUIDs are hidden exactly like nonexistent values.
- Parent queue/filter/history/report access never leaks other households; archived behavior matches this contract.
- Response, audit, and log inspection proves absence of secrets, raw database errors, sibling data, parent identifiers in child projections, unrestricted metadata, and unsafe reason rendering.

### Frontend and accessibility

- Component tests cover populated/multi-child and empty queues, approve, rejection guidance, reversal confirmation, correction confirmation, balance/activity, 30-day history, and all loading/error/offline/permission/locked/conflict states.
- Ambiguous replay reuses key/body/version; a new action uses a new key and refreshed version; duplicate taps cannot cause a second request.
- Tests assert live announcements, initial/restored/moved focus, keyboard operation, accessible names, headings/list semantics, busy/disabled state, error association, and non-color status text.
- Responsive/manual checks cover narrow phone, tablet, desktop, long names/titles/reasons, 200% zoom, reduced motion, and practical target sizes.
- Backend format/test/vet/build, PostgreSQL integration and concurrency tests, OpenAPI validation, frontend format/lint/typecheck/tests/accessibility scan/build, Docker rebuild, and live API journeys all pass.

## Phase 7 release gate

Phase 7 is complete only when:

- all blocking contract corrections match the OpenAPI, migrations, service, and clients;
- migrations apply from an empty PostgreSQL 16 database and ledger invariants reject hostile direct writes;
- live submit-to-approve, submit-to-reject-to-resubmit, approve-to-reverse, and manual-correction journeys reconcile occurrence, attempt, ledger, audit, idempotency, balance, history, and report data;
- concurrency demonstrates exactly one award and one possible reversal under repeated and competing requests;
- at least 30 local days of parent history and daily/weekly/monthly report APIs pass boundary and reconciliation tests;
- tenant isolation, archived-child policy, child-safe projections, and reason privacy are verified;
- parent queue, child points/activity, and parent history meet the error-state and accessibility acceptance above;
- all backend, frontend, OpenAPI, Docker, health, and regression checks pass with no critical/high security or data-integrity finding.

The roadmap gate must remain unchecked until validation evidence demonstrates every condition. Phase 8 may build the household overview and polished report visualizations on these stable report semantics.
