# Phase 9.1 Periodic Reward Eligibility Contract

Status: **approved for implementation; release gate open**
Date: 2026-08-18
Scope: an optional, household-defined qualification period layered onto the Phase 9 reward catalog and redemption ledger.

This contract is normative for Phase 9.1. It separates achievement from spending: a child qualifies by earning enough eligible points during a closed daily, weekly, or monthly collection period, while the existing signed ledger balance remains the only amount they can spend. Points and balances never reset at a period boundary.

## Product outcome and defaults

Parents may enable periodic eligibility only when rewards are enabled. The default remains **off**, so every existing household and reward behaves exactly as in Phase 9 until a parent opts in.

The first release supports these AND-combined rules:

- required minimum net approved points;
- optional minimum completion percentage; and
- optional maximum number of redemption requests in the eligibility window.

Recommended initial policy is weekly, the household's configured week start, 100 minimum points, 24-hour approval grace, no completion rule, and one request per eligibility window. The UI must present these as editable recommendations, not universal values.

## Terms and invariants

- **Collection period** is a closed-open household-local interval `[startsAt, endsAt)` in which work occurs.
- **Evaluation cutoff** is `endsAt + approvalGrace`. Evaluation becomes final at or after that instant.
- **Eligibility window** is the immediately following matching calendar period. A passing child may redeem from evaluation finalization until that following period ends.
- **Period score** is qualifying net approved points for occurrences whose immutable `local_date` falls in the collection period. It is not the ledger balance.
- **Current balance** remains the sum of all signed ledger entries. It never resets and must cover the reward cost at request time.

An eligible child with insufficient current balance cannot redeem. A child with sufficient balance but no passing current evaluation cannot redeem when periodic eligibility is enabled. Reward availability, active state, household feature flags, child state, reward version, eligibility, redemption cap, and current balance must all pass in the same authoritative request transaction.

## Calendar boundaries

All boundaries use the policy version's snapshotted IANA household timezone. Local dates are resolved with the existing timezone utilities; UTC instants are derived from local midnight. A DST day may contain 23 or 25 hours and must not be treated as a fixed 24-hour UTC interval.

- `daily`: one household-local calendar date, local midnight to the next local midnight.
- `weekly`: seven local dates beginning on the snapshotted `week_starts_on` (`0` Sunday through `6` Saturday), local midnight to local midnight seven dates later.
- `monthly`: the first local date of a calendar month through the first local date of the next month.

Changing the household timezone or week start never changes a collecting or evaluated period. New policy versions snapshot the new values and apply only at a future boundary.

Approval grace is one of `0`, `12`, `24`, or `48` hours for weekly/monthly policies, defaulting to 24. Daily policies require zero grace in Phase 9.1 because a 24-hour grace would eliminate the next-day window. Grace is elapsed time after the boundary instant, not additional local dates.

The eligibility window is the calendar period immediately following the source collection period using that policy version's same frequency and boundary settings. It begins at the source period's `endsAt`; however, redemption is locked until evaluation finalizes. It expires exclusively at the following period's `endsAt`. Consequently weekly/monthly grace shortens the usable window by the selected grace. No eligibility carries forward or stacks.

## Policy versions and effective dates

Persist a household-owned policy with immutable versions. A version contains frequency, minimum points, optional completion percentage, optional redemption cap, approval grace, timezone, week start, and effective period boundary. Values are validated as:

- minimum points: integer `1..1,000,000`;
- completion percentage: absent or integer `1..100`;
- redemption cap: absent or integer `1..100` requests;
- frequency: `daily | weekly | monthly`.

Create/update/disable requires Parent Mode, CSRF, idempotency, `If-Match`, and audit. The server computes and returns `effectiveFrom` as the next valid boundary using the requested frequency and the household calendar snapshot; clients never calculate or submit this security-sensitive boundary. A version never edits an already collecting, closed, or evaluated period. There may be at most one not-yet-active scheduled version. A later parent edit may replace that pending version because it has never owned a collection period; the audit trail retains both scheduling actions. Disabling schedules the same kind of future replacement and does not delete active policy history, evaluations, ledger entries, or redemption history.

There is at most one applicable version per household boundary. A frequency change starts a fresh collection window at its effective boundary; it never evaluates dates using the previous frequency, and redemption remains locked until the new version completes its first evaluation. The service materializes periods idempotently and records the exact policy version. Policy creation does not retroactively evaluate older work. Existing requested redemptions remain parent-decidable after any policy change or disable.

## Qualifying points and completion

Minimum points uses **net approved occurrence awards**:

1. Include an `award` only when its occurrence's immutable local date is inside the source period and the award exists by the evaluation cutoff.
2. Include the award even if the parent approved it during the allowed grace.
3. Subtract its linked `approval_reversal`; reward redemption/refund entries never affect period score.
4. Exclude manual corrections by default; Phase 9.1 has no UI to include them.
5. Exclude pending, withdrawn, rejected, cancelled, or unapproved work.

Approval time alone never moves work into another collection period. Occurrence local date controls attribution. The final evaluation snapshots qualifying award IDs, reversal IDs, totals, rule inputs, and results so it is explainable and reproducible.

Optional completion percentage is calculated as `approved eligible occurrences / expected eligible occurrences * 100`, without rounding up. Expected occurrences are the child's noncancelled habit/task occurrences assigned for dates in the period; approved occurrences are those with a valid award by cutoff. An empty denominator produces 100%, avoiding punishment when no work was assigned. Points and completion rules are AND-combined.

## Evaluation, late changes, and eligibility

Evaluation is an idempotent server operation triggered opportunistically on relevant reads/redemption plus a scheduled worker in production. Correctness cannot depend on the worker running at an exact instant. Only one final evaluation may exist for `(family, child, source period, policy version)`.

Before cutoff, progress is provisional and shows approved-to-date values. At or after cutoff, evaluation locks the child and source period, reads authoritative occurrences and ledger links, writes immutable rule results, and records `eligible` or `not_eligible`, finalization time, and eligibility expiry.

- Approval after cutoff does not alter the final score or grant eligibility. It still affects lifetime balance and is labelled as approved too late for that period.
- An approval reversal after finalization creates an immutable evaluation adjustment linked to the original evaluation. The system recomputes the rule outcome for transparency.
- If the adjusted result no longer passes and no request has used the window, unused eligibility is revoked immediately.
- If a request already exists, it and any parent decision remain valid; no automatic debit, refund, negative balance, or retroactive cancellation occurs. Further requests in that window are blocked.
- Re-approval after cutoff does not restore that window. Administrative re-evaluation is out of scope; corrections require a future audited feature.

The child is eligible only while the evaluation is passing, not revoked, finalized, and `now < eligibilityEndsAt`. Archival locks the child out without rewriting evaluations. A household may have only one current eligibility window for a child; changing frequency cannot overlap active versions.

The optional redemption cap counts successfully created `requested` redemptions, including ones later fulfilled or cancelled. Cancellation refunds balance but does not restore the cap, preventing request/cancel cycling. Cap enforcement is household-wide across all rewards for that evaluation window.

## Persistence contract

Use household-scoped structural keys throughout. Recommended resources are:

- `reward_eligibility_policies`: household identity, active version pointer, version, timestamps.
- `reward_eligibility_policy_versions`: immutable configuration, snapshotted timezone/week start, effective boundary, actor/audit metadata.
- `reward_collection_periods`: policy version, local start/end dates, UTC boundary/cutoff instants, state.
- `reward_period_evaluations`: family, child, source period, totals, status, finalized/revoked timestamps, eligibility start/end, version.
- `reward_evaluation_rule_results`: typed target, actual, pass/fail, explanatory metadata.
- `reward_evaluation_adjustments`: immutable post-finalization reversal effects.

Each redemption created under periodic eligibility snapshots and structurally references exactly one passing evaluation. Existing Phase 9 redemptions may have a null evaluation reference. Database checks/triggers must enforce same family and child. No table may update or delete the append-only point ledger.

## API contract

Exact routes may follow existing conventions, but OpenAPI must expose:

- parent get/create/update/disable policy with current and scheduled versions;
- parent per-child current progress and paginated evaluation history;
- child current progress/eligibility projection with no sibling data;
- child catalog fields `eligibilityRequired`, `eligible`, `eligibilityEndsAt`, `redemptionsUsed`, `redemptionsAllowed`, and rule-level `target`, `actual`, `passed`, `shortfall`;
- stable redemption failures: `eligibility_not_final`, `eligibility_not_met`, `eligibility_expired`, `redemption_limit_reached`, and the existing balance/availability/version failures.

Server projections, not clients, compute dates, score, eligibility, shortfalls, and `canRedeem`. Progress and private responses use `Cache-Control: no-store`. Cursors are opaque, tenant-bound, filter-bound, stable under tied timestamps, and reject malformed/cross-query reuse. Unknown JSON fields and invalid dates/configuration fail without writes.

## Parent and child experience

Parent settings explain that qualifying does not spend or reset points and that children still need the reward cost in current balance. Parents select period, threshold, applicable grace, optional completion target, optional request cap, and effective boundary. A preview gives concrete local dates for collection, evaluation, and redemption. Parent progress/history shows every rule and late/reversed adjustments without competitive ranking.

Child progress uses supportive language:

- collecting: `75 / 100 points · 25 more to qualify` plus period end;
- awaiting cutoff: `This period ended; waiting for parent approvals`;
- eligible: `Rewards unlocked until …` plus current balance and cap usage;
- not eligible: `You earned 85 of 100 points. A new period has started.`

Rewards remain visible when the catalog is enabled, with disabled request controls and specific, non-blaming reasons. Distinguish qualification shortfall, current-balance shortfall, expired window, and exhausted cap. Never use “failed”, streak pressure, leaderboards, sibling comparisons, or punishment language.

## Authorization, privacy, and concurrency

- Only an unlocked same-household parent may mutate policy or read all-child progress/history.
- Child Mode may read only its exact active child's progress, rule results, catalog, and redemptions. It cannot enumerate sibling policy outcomes, balances, identifiers, or timing differences.
- Shared/locked/expired/revoked sessions, archived children, cross-household IDs, and role mismatch follow the existing concealed-resource policy.
- Every mutation is CSRF-protected, idempotent, audited, versioned, size-limited, and safely logged. Audit records contain configuration/rule outcomes but no PIN, cookie, CSRF token, or raw request secret.
- Redemption locks in fixed order: family/policy version, child balance serialization row, evaluation, reward, then cap usage/redemption. Evaluation and reversal paths use the compatible family/child/evaluation order.
- Under one serializable result, recheck feature flags, policy applicability, unexpired passing evaluation, cap, reward availability/version/cost, and signed balance; then create redemption, evaluation link, debit, audit, and idempotency response.
- Concurrent evaluation creates one result. Concurrent requests cannot exceed either balance or cap. Races with cutoff, reversal, archive, policy/reward disable, correction, fulfillment, or cancellation produce one legal winner, coherent loser errors, no deadlock, and no partial writes.

## Test and release gate

Phase 9.1 remains open until evidence covers:

- fresh and production-shaped PostgreSQL migration/rollback safety without changing historical balances/redemptions;
- daily, both week starts, month/year edges, Berlin/Jakarta timezones, DST transitions, 23/25-hour days, and timezone/week-start policy changes;
- every grace value, exact cutoff races, late approval, pre/post-finalization reversal, empty workload, exact threshold, one short, and completion percentages without upward rounding;
- policy version scheduling, enable/disable, frequency changes, no overlap/retroactivity, idempotent materialization/evaluation, and immutable snapshots;
- cap zero-remaining behavior, cancellation not restoring capacity, exact/insufficient balance, concurrent cap/balance requests, and reconciliation of evaluation score versus signed ledger balance;
- hostile direct writes for cross-family/child/policy/evaluation links, duplicate finals, overlap, invalid boundaries/configuration, and mutation of immutable results;
- the full parent/child/archived/wrong-child/cross-household/session/CSRF/idempotency/version authorization matrix and private cache/log projections;
- frontend unit/component tests for every progress and failure state, keyboard/focus/live-region behavior, 320px reflow, 200% zoom, forced colors, reduced motion, screen reader, and authenticated axe checks;
- an authenticated browser/PostgreSQL journey: configure future weekly policy, collect and approve work including grace, finalize evaluation, redeem within cap and balance, reject concurrent excess, reverse a qualifying award, and reconcile history/reporting;
- OpenAPI validation, Go format/test/vet/build/race where applicable, frontend format/lint/typecheck/test/build, Docker health, security scans, SBOM, and Phase 3–9 regressions with no unresolved critical/high security, privacy, accessibility, concurrency, or data-integrity defect.

Production launch remains blocked on the Phase 8, Phase 9, and Phase 9.1 gates.

## Explicitly deferred

- Arbitrary nested AND/OR rule builders, required-habit rules, per-reward thresholds, and per-child custom policy values.
- Points reset/expiry, protected minimum balances, automatic punishment/deductions, leaderboards, and streaks.
- Cap restoration after cancellation, rolling windows, custom-length periods, retroactive policy creation, and manual re-evaluation.
- Notifications, inventory/stock, delivery integrations, and automatic reward fulfillment.
