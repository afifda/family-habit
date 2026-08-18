# Phase 9 Dynamic Routines and Reward Redemption Contract

Status: **implementation-ready review**
Date: 2026-08-18
Scope: the approved Phase 9 routine-groups and reward-redemption plan, extending PR-04 through PR-09.

This contract refines the approved proposal against the current PostgreSQL schema, append-only ledger, household authorization boundary, effective-dated assignments, occurrence snapshots, and parent/child UI. Its decisions are normative for Phase 9. Existing workflow groups (`to_do`, `waiting_for_parent`, and `done`) remain occurrence states/filters; they are not routine groups.

## Release outcome

Parents can define and order optional household routine groups, place each child's recurring assignment or one-off task in a group, and see stable group snapshots in occurrence history. Child Today is organized into those routines while preserving workflow filters and an automatic **Other** section.

When the household opt-in is enabled, parents can define positive-cost rewards and eligible children. A child can reserve points by requesting an eligible reward without ever overspending; a parent can fulfill the request or cancel it with an exact refund. Every point movement is an immutable, attributable, tenant-consistent ledger entry, and reports reconcile earned, redeemed, refunded, corrections, and net change.

## Blocking implementation corrections

1. The current ledger requires `actor_user_id` for every entry. A redemption initiated in Child Mode must not be falsely attributed to the owning parent account. Add nullable `actor_child_id`, make `actor_user_id` nullable, enforce exactly one actor, and add household-scoped actor foreign keys. Existing rows retain their parent actor. `reward_redemption` uses the active child actor; `reward_refund` uses the deciding parent actor.
2. Routine membership changes for recurring work must follow the existing effective-dated assignment model. Do not mutate an assignment in place when that would change generated history. A change effective on local date D closes the prior assignment at D−1 and creates its replacement, including schedule, points, routine group, and item order.
3. Reward availability cannot infer “all children” from an empty join table because an intentionally empty selected catalog becomes indistinguishable. Persist an explicit availability scope of `all_active_children` or `selected_children`; use the former as the UI default and require at least one active, same-household join row for the latter.
4. Disabling rewards or archiving a reward must not strand existing requests. It prevents new child requests and hides the catalog item, but parents can still read and decide every existing `requested` redemption.
5. Reward debits/refunds must be structurally linked to a redemption. Reusing the generic reversal link alone is insufficient because approval reversals and reward refunds have different state and actor rules.

## Routine group domain

### Resource and validation

`routine_groups` is household-owned and contains `id`, `family_id`, trimmed `name`, nullable `icon`, nullable `color`, nullable `starts_at_local`, nullable `ends_at_local`, integer `sort_order`, positive `version`, `archived_at`, and timestamps.

- Name is 1–60 Unicode characters after trimming. Active names are unique per household on `lower(btrim(name))`; archived names do not block reuse.
- Icon follows the existing safe icon allowlist and color follows the existing color validation. Neither conveys meaning alone.
- Time hints use PostgreSQL `time without time zone`, serialize as `HH:mm`, and are interpreted in the household's current IANA timezone. Both may be absent. If supplied, both are required and cannot be equal. `end < start` means the display window crosses midnight. Windows never hide, unlock, expire, reorder, or authorize work.
- Active `sort_order` is unique per household and dense in the service response. It is presentation data, not a clock-derived priority.
- **Other** is virtual, always last, has no ID, cannot be edited, and contains every item whose group snapshot is null. Clients must not manufacture a UUID for it.

The product offers Morning, Afternoon, and Evening as an explicit parent-confirmed starter action only. Registration and migration create no groups automatically.

### Membership and effective dating

Add nullable `routine_group_id` and nonnegative `sort_order` to `habit_assignments` and `one_off_tasks`. Every non-null relationship is structurally scoped to the same household. Group membership belongs to the child-specific assignment, not the shared habit definition.

Changing a recurring membership/order uses the same local effective-date contract and overlap protection as points/schedule changes. Generated occurrences before D remain untouched. Ungenerated dates from D use the replacement assignment. Changing a task group/order is allowed only while its occurrence is `not_started` with no attempt; task and its occurrence snapshot update atomically under the existing task version check. A progressed task keeps its snapshot and cannot be moved.

Archiving an in-use group requires `moveToRoutineGroupId`, whose value is either another active same-household group or explicit `null` for Other. In one parent transaction the service locks the group and affected active assignments/tasks, validates the destination, performs effective-dated assignment replacement using the required household-local `effectiveFrom`, moves editable tasks, archives the group, compacts active ordering, audits, and finalizes idempotency. If any in-use progressed task cannot be moved, return `409 group_in_use` with a count and make no writes; the parent may leave it historical by first completing/cancelling it, but archival never rewrites its occurrence.

### Occurrence snapshot and ordering

Occurrences add nullable `routine_group_id_snapshot`, `routine_group_name_snapshot`, `routine_group_icon_snapshot`, `routine_group_color_snapshot`, `routine_group_sort_order_snapshot`, and nonnegative `item_sort_order_snapshot`. Snapshot shape is all-null for Other, or has ID/name/group order together with nullable icon/color. The existing snapshot immutability trigger covers these columns after an occurrence has progressed or has an attempt.

Generation copies the assignment/task values transactionally. Existing occurrences remain null/Other. A generated, unstarted occurrence may be safely regenerated only under the existing Phase 5 rules; progressed history is never rewritten.

Today returns server ordering by `routine_group_sort_order_snapshot ASC NULLS LAST`, `item_sort_order_snapshot ASC`, due context, title, then occurrence ID. It exposes routine sections even when a workflow filter is applied, omitting empty sections except that an entirely empty result shows one clear empty state. Workflow filters are `all`, `to_do`, `waiting_for_parent`, and `done`; filtering never changes membership or counts. History returns the immutable routine projection but does not use current group definitions.

### Routine state machine

| Current state | Parent action | Result |
|---|---|---|
| absent | create | active version 1 |
| active | edit presentation/time hint | active, version +1; history unchanged |
| active | reorder complete active set | all affected active groups version +1 in one transaction |
| active in use | archive with valid destination/effective date | assignments/tasks moved per rules; archived, version +1 |
| active not in use | archive | archived, version +1 |
| archived | any mutation | `409 invalid_state_transition` |

Restoration and hard deletion are out of scope. Archived groups are parent-readable only where required to label administrative history; children receive occurrence snapshots, never archived definitions.

## Reward catalog and household setting

Add `rewards_enabled boolean NOT NULL DEFAULT false` to `families`. The existing household settings update accepts it only in Parent Mode with CSRF, idempotency, and the household version/conditional-write convention. Turning it off creates no ledger or redemption changes.

`rewards` contains `id`, `family_id`, trimmed `title`, nullable `description`, nullable `icon`, positive `cost_points`, availability scope, positive `version`, `archived_at`, and timestamps. Title is 1–80 characters, description at most 500, cost is 1–10,000. The selected-child join is unique by `(reward_id, child_id)` and structurally same-household. Only active children may be newly selected. Archived children remain linked for historical integrity but never receive a catalog.

Parent create/update/archive and eligibility replacement are atomic, audited, CSRF-protected, idempotent mutations. Updates and archival require `If-Match`. Cost/title changes affect only later requests; redemptions snapshot both. Archival is soft and does not alter requested or decided redemptions. Restoration, inventory, expiry, limits, child-authored rewards, and hard deletion are out of scope.

Child catalog visibility requires all of: Child Mode for the exact active child, unarchived child, rewards enabled, active reward, and eligibility through `all_active_children` or a matching join. `canRedeem` and `shortfallPoints` are computed from one transactionally consistent balance/catalog read; they are guidance only and never replace mutation-time validation.

## Redemption and ledger domain

### Persistence

`reward_redemptions` contains household, child, reward, immutable title/cost/icon snapshots, state, requested child actor/time, nullable deciding parent/time, nullable cancellation reason, positive version, and timestamps. It has household-scoped foreign keys and a unique identity suitable for ledger linkage.

Extend ledger kinds with:

- `reward_redemption`: amount equals `-cost_points_snapshot`; links one redemption; has no occurrence, completion, reversal, or reason; actor is the requesting child.
- `reward_refund`: amount equals the positive cost snapshot; links the same redemption and its original debit; has no occurrence/completion; actor is the cancelling parent; its required reason is the normalized cancellation reason.

There is exactly one debit per redemption and at most one refund per redemption/debit. A database trigger or equivalent constraint validates kind, sign, exact amount, family, child, redemption state/link, original debit kind, and actor type. Ledger rows remain append-only. No generic negative correction or punishment operation is added.

### State machine

| Current state | Actor/action | Preconditions | Atomic result |
|---|---|---|---|
| absent | child requests | feature enabled; active/eligible reward and child; reward version matches; authoritative balance ≥ cost | `requested` version 1, immutable snapshot, one exact negative debit, child audit, finalized idempotency |
| `requested` | parent fulfills | redemption version matches | `fulfilled`, version +1, parent/time, no ledger change, audit, finalized idempotency |
| `requested` | parent cancels | version matches; reason 1–500 trimmed characters | `cancelled`, version +1, parent/time/reason, one exact positive refund, audit, finalized idempotency |
| `fulfilled` | any transition | none | `409 invalid_state_transition`, no writes |
| `cancelled` | any transition | none | `409 invalid_state_transition`, no writes |

Children cannot cancel, fulfill, repeat, or edit a request. Parents cannot create a redemption on a child's behalf in this release. Exact replay is the only success after state advancement. Fulfillment deliberately makes no balance change because points were reserved at request time.

### Serialization and rollback

All redemption-affecting transactions lock in this fixed order: household/feature state when needed, child row (the balance serialization boundary), reward row, redemption row if present, then referenced debit. Request locks the child before calculating `SUM(point_ledger.amount)` for `(family_id, child_id)`. Different-key concurrent requests therefore compete against one authoritative balance and no committed redemption may make it negative.

The redemption/debit/audit/idempotency result is one transaction. Cancellation/refund and fulfillment follow the same atomic rule. Same-key concurrent requests wait and replay the stored response or return the established retryable in-progress response; changed reward/version/body with the same key returns `409 idempotency_conflict`. Different keys may create distinct requests only when the locked balance funds all of them. Faults never leave an orphan redemption, debit, refund, audit event, or permanently executing key.

Parent positive correction and approval/reversal operations must use the same child balance serialization boundary after this migration so a correction or reversal cannot race a redemption into an inconsistent balance. Approval reversal may reduce balance only by undoing an existing award; if reserving rewards means the exact reversal would make the balance negative, it returns `409 insufficient_available_points` and does not transition. This is a necessary safety rule: a spent award cannot be reversed until enough points are restored through legitimate positive entries or a pending redemption is cancelled.

## HTTP API projections

All response envelopes, error shape, CSRF cookie/header policy, UUID validation, body limit, request IDs, and safe 404 behavior follow the existing API. Mutation keys use 8–128 visible characters. `If-Match` is the quoted positive numeric version accepted by existing mutation families.

### Parent routine API

- `GET /api/v1/routine-groups?includeArchived=false` → ordered `RoutineGroup[]`; Parent Mode only.
- `POST /api/v1/routine-groups` with name/presentation/time hints → `201`; idempotency required.
- `PATCH /api/v1/routine-groups/{id}` → updated group; `If-Match` and idempotency required.
- `PUT /api/v1/routine-groups/order` with `{orderedIds:[...]}` → complete ordered active projection; `If-Match` is not used because every active ID/version pair is instead required in `{items:[{id,version}]}`; omission, duplicate, archived, or foreign ID is `400`/safe `404`; idempotency required.
- `POST /api/v1/routine-groups/{id}/archive` with `{effectiveFrom,moveToRoutineGroupId}` → archived projection; `If-Match` and idempotency required.
- Existing assignment create/update and task create/update add nullable `routineGroupId` and `sortOrder`. Assignment move requires `effectiveFrom`; task update keeps current state/version rules.

`RoutineGroup` exposes ID, name, icon, color, `startsAtLocal`, `endsAtLocal`, sortOrder, version, archivedAt. Child APIs never expose definition version or archival metadata.

### Reward API

- `GET /api/v1/rewards?includeArchived=false` → parent catalog with scope and eligible child IDs; Parent Mode only.
- `POST /api/v1/rewards` → `201`; idempotency required.
- `PATCH /api/v1/rewards/{id}` → update presentation/cost/scope/full selected child set; `If-Match` and idempotency required.
- `POST /api/v1/rewards/{id}/archive` → archived projection; `If-Match` and idempotency required.
- `GET /api/v1/child/rewards` → exact active child's safe catalog and balance summary; Child Mode only.
- `POST /api/v1/child/rewards/{rewardId}/redemptions` with `{rewardVersion,confirmedCostPoints}` → `201 Redemption`; Child Mode, CSRF, idempotency required. The confirmed cost must equal the locked current reward cost or return `409 version_conflict` without writes.
- `GET /api/v1/reward-redemptions?state=requested&childId=` → parent queue with deterministic keyset ordering `(requested_at ASC,id ASC)`; Parent Mode only.
- `GET /api/v1/child/reward-redemptions?cursor=` → exact child's own history `(requested_at DESC,id DESC)`; Child Mode only.
- `POST /api/v1/reward-redemptions/{id}/fulfill` → updated redemption; Parent Mode, CSRF, `If-Match`, idempotency.
- `POST /api/v1/reward-redemptions/{id}/cancel` with required reason → updated redemption plus refund linkage; Parent Mode, CSRF, `If-Match`, idempotency.

The child reward projection contains reward ID/version, title, description/icon, cost, `canRedeem`, and shortfall. It contains no eligible-child list, sibling ID/balance, family ID, archive metadata, or parent identity. Redemption contains ID, immutable safe reward snapshot, signed reserved/refunded effects, status, timestamps, and version; child projection omits deciding user ID and administrative audit metadata. Parent projection may include child presentation and cancellation reason but never PIN/session/security data.

Expected domain errors include `rewards_disabled`, `insufficient_points`, `reward_unavailable`, `group_in_use`, `version_conflict`, `invalid_state_transition`, and `idempotency_conflict`. Cross-household and wrong-child resources remain indistinguishable from nonexistent resources.

## Reporting and reconciliation

Existing occurrence metrics are unchanged. Report point fields become:

- `pointsEarned`: awards plus approval reversals attributed to immutable occurrence local dates, retaining Phase 7 semantics;
- `manualCorrections`: positive corrections attributed by `created_at` in household timezone;
- `pointsRedeemed`: positive magnitude of reward-redemption debits attributed by debit `created_at` in household timezone;
- `pointsRefunded`: positive reward refunds attributed by refund `created_at` in household timezone;
- `netPointsChange = pointsEarned + manualCorrections - pointsRedeemed + pointsRefunded`.

The current balance remains the lifetime signed sum of the ledger and is not expected to equal a bounded-period net change. Reports do not retroactively move a debit when a reward is fulfilled/cancelled, do not materialize data, and use transactionally consistent reads. Child activity labels are neutral: **Reward requested** and **Reward refunded**. Parent ledger/history links the redemption and snapshot without exposing sibling information.

## Authorization, privacy, audit, and logging

- Routine/catalog/settings management, parent redemption queue, fulfill, cancel, and parent reports require an active unlocked Parent Mode owner for the server-derived family. UI visibility is never authorization.
- Child catalog/request/history require Child Mode and the session's exact active, non-archived child. No child ID is accepted from child request bodies or query filters.
- Shared, locked-parent, wrong-mode, inactive, archived-child, revoked/expired, unauthenticated, wrong-child, and cross-household cases fail without writes. Mutation CSRF failure occurs before idempotency/business writes.
- Valid foreign UUIDs, filters, cursor reuse, group destinations, eligibility child IDs, and reward/redemption IDs use safe non-enumerating behavior. Batch eligibility and reorder operations are all-or-nothing.
- Audit events record allowlisted IDs, versions, signed amount/cost, state transition, safe scope, session, actor, and idempotency key. Child audit uses `actor_child_id`; parent audit uses `actor_user_id`. Free-form descriptions, nicknames, secrets, headers, raw bodies, cookies, PIN/password/session/CSRF values, and unrestricted metadata are excluded.
- Cancellation reason is parent administrative text. Child UI receives only the neutral fact and refund amount, not the reason or deciding parent. All free text is length-limited and rendered as text, never HTML.
- Logs use request IDs and safe action/error codes; they contain no titles/descriptions/reasons, child names, balances, secrets, raw database errors, request bodies, or cross-household data.

## UX and accessibility acceptance

### Parent routines and catalog

- Habits & Tasks provides a clearly named routine manager plus group selection/order per child assignment and task. It explains that time windows are hints and that earlier occurrences keep their original routine.
- Drag-and-drop is optional enhancement only. Every ordering action has visible keyboard-operable **Move up**, **Move down**, and **Move to routine** alternatives and announces the new position. Focus remains on the moved control.
- Archival names the affected item counts, requires an explicit Other/destination choice and effective date, and explains that history is unchanged. It handles stale/in-use conflicts without pretending success.
- Rewards settings clearly show the off-by-default household switch. Catalog forms expose exact positive cost and all/selected eligibility. Disabling/archiving warns that existing requests remain in the parent queue.

### Child Today and rewards

- Today defaults to routine sections in household order and Other last. Each section has a real heading and item count. Status is always text and is filterable as All, To do, Waiting, and Done; color/icon/time hints are supplementary.
- The Rewards entry point is absent when disabled. Enabled empty, insufficient-points, loading, offline/error/retry, and populated states have neutral copy and no sibling comparison, countdown, scarcity, streak, or leaderboard mechanics.
- Request confirmation names the reward, current balance, exact `−N points`, and resulting balance. The confirm control cannot be enabled from stale client arithmetic alone. One activation disables duplicate action and exposes busy state; an ambiguous retry reuses the exact key/body/version.
- Parent queue identifies child, immutable reward/cost, reserved effect, and request time. Fulfill states that points are already reserved. Cancel confirmation states exact `+N points`; cancellation reason is labelled parent-only.
- Success updates balance/catalog/activity/queue caches, announces through a polite live region, and moves/restores focus predictably. Conflicts refresh authoritative data and explain the non-punitive outcome.

All flows implement loading, empty, success, validation, offline/recoverable error, locked/permission loss, stale conflict, and retry states. Controls have visible focus, semantic names, error association, busy/disabled state, keyboard operation, and practical 48×48 CSS-pixel targets. Dialog focus is trapped/restored and Escape behavior is safe. Content works at 320 CSS pixels, 200% text zoom, portrait/landscape, reduced motion, forced colors, and screen readers. Long translated names/titles and large point values neither clip nor make actions unreachable.

## Migration and rollback compatibility

The forward migration must apply both to an empty PostgreSQL 16 database and a production-shaped Phase 8 database.

- Existing families receive `rewards_enabled=false`.
- Existing assignments/tasks receive null group and deterministic default item order; all existing occurrences receive null routine snapshots and therefore render under Other.
- Existing ledger rows retain kind, signed amount, parent actor, timestamp, balance, and ordering. Adding child actor support does not rewrite them.
- New enum values are added safely. Down migration must account for PostgreSQL enum-value removal limitations: it may refuse while reward ledger rows/redemptions exist or rebuild/cast the enum only after an explicit data-safety check. It must never silently delete ledger/history to roll back.
- Index/constraint creation is ordered so invalid partial state cannot become visible. Migration validation compares per-child ledger row count and signed balance before/after, occurrence snapshot values, assignment/task counts, and household flags.
- The old application must not be deployed against a database containing new ledger kinds. Deployment is migrate-then-new-API as one maintenance release with a verified backup and restore procedure. Routine/reward UI is exposed only after the compatible API reports readiness.

## Required release test matrix

### Routine model and history

- Create/edit/reorder/archive, case-insensitive active-name uniqueness, input bounds, crossing-midnight windows, equal/partial window rejection, dense deterministic order, complete-list reorder, stale versions, exact replay, and mismatched-key reuse.
- Same habit assigned to two children in different routines remains independent. Effective-dated move closes/replaces only the selected child's assignment and preserves schedule/points.
- Task move succeeds only while editable. Archive-to-Other/destination is atomic; a progressed in-use task yields `group_in_use` and zero writes.
- Existing and progressed occurrences keep null/old snapshots after group create/rename/reorder/archive/move. New occurrences receive correct immutable snapshots. Other is last and not persisted as a fake resource.
- Today ordering and every workflow filter pass with multiple routines, identical sort/title values, overdue tasks, empty groups, no groups, archived definitions, and timezone changes.

### Redemption, ledger, and reconciliation

- Disabled household exposes no child catalog/request capability. Enable/disable, all/selected/empty eligibility, child archive, reward edit/archive, and historical requests behave exactly as specified.
- Request at exact balance succeeds and reaches zero; one point short fails with no write. Snapshot cost/title survive later reward edits/archive.
- Fulfillment changes state/version/audit only. Cancellation creates one exact refund and preserves the debit. Terminal states reject every further transition.
- Database-hostile inserts reject wrong signs, wrong amounts, missing/multiple actors, parent-as-child debit, child-as-parent refund, duplicate debit/refund, mismatched family/child/reward/redemption/original debit, mutable/deleted ledger rows, and invalid kind/link shapes.
- Balance equals the signed ledger after mixed awards, approval reversals, corrections, redemptions, refunds, fulfillments, and cancellations. Period reports satisfy the defined equation at Jakarta/Berlin midnight, DST boundaries, week/month/year edges, and tied timestamps.

### Idempotency, concurrency, rollback

- Sequential/concurrent exact replay for every routine/catalog/settings/request/decision mutation returns the stored response with one effect. Same key with changed target, version, order, destination, eligibility, cost, or reason conflicts with no writes.
- Concurrent different-key requests at one-child exact/insufficient balances never overspend; only the fundable set commits. Duplicate requests are distinct only when separately funded and separately confirmed.
- Request races approval, correction, approval reversal, reward edit/archive, feature disable, child archive, and cancellation. Lock ordering produces a legal winner, no deadlock, no negative committed balance, and coherent loser errors.
- Fulfill/cancel, cancel/cancel, reorder/edit, archive/move, and assignment move/generation races each have one coherent result.
- Fault injection after every write stage proves total rollback and safe retry, including idempotency finalization.

### Authorization and privacy

- Every operation covers unauthenticated, expired/revoked, shared, locked parent, child, wrong child, archived child, parent, nonexistent, and valid cross-household resources as applicable.
- Child A cannot learn Child B's catalog eligibility, balance, request history, reward availability difference, or identifiers. Parent batch filters never cross the session household.
- CSRF, malformed/oversized JSON, unknown fields, invalid UUID/version/key/cursor, and rate/request limits cause no business writes.
- API, browser cache, audit, and log inspection proves projections and exclusions in this contract, safe text rendering, `no-store` for private data, and absence of secrets or raw persistence errors.

### Frontend, accessibility, and journeys

- Component/interaction tests cover every state and behavior in the UX sections, including keyboard reorder, focus after moves/decisions, duplicate activation, exact retry keys, conflict refresh, live announcements, and long content.
- Automated axe checks cover populated/empty/error routine Today, routine manager dialogs, catalog/editor, request confirmation, and parent queue/decision dialogs in authenticated modes with no serious/critical findings.
- Keyboard-only and screen-reader checks complete parent create/reorder/archive and child request plus parent fulfill/cancel. Phone/tablet/desktop, 320px reflow, 200% zoom, forced colors, reduced motion, and target-size evidence is recorded.
- A live PostgreSQL/browser journey performs: parent enables rewards; creates/reorders routines and assigns work; creates eligible reward; child earns approved points; child requests; concurrent overspend is rejected; parent fulfills one and cancels another; child balance/activity and parent reports reconcile; group rename/archive leaves history stable.

### Engineering regression

- Empty and production-shaped migrations plus rollback safety checks pass on PostgreSQL 16.
- Backend format, unit/integration/concurrency/race where applicable, vet, and builds pass; OpenAPI validates and generated/client types match behavior.
- Frontend format, lint, TypeScript, unit/component, browser accessibility, and production build pass.
- Docker rebuild, health/readiness, security headers, dependency/filesystem/image scans, SBOM generation, and existing Phase 3–8 regression journeys pass with no unresolved critical/high security or data-integrity finding.

## Phase 9 release gate

Phase 9 is complete only when:

- the blocking corrections and all database constraints above are implemented and proven by hostile direct-write tests;
- production-shaped migration places existing work under Other without changing a single historical snapshot, ledger row, or balance;
- routine CRUD/order/archive, effective-dated membership, occurrence generation, Today filters, and immutable history pass backend and authenticated browser tests;
- rewards remain off by default, eligibility is private, and requested → fulfilled/cancelled is the only lifecycle;
- transaction/concurrency tests prove no negative redemption balance, duplicate debit/refund, orphan state, false actor attribution, or deadlock;
- reports and child/parent history reconcile every ledger kind using the specified time attribution and equation;
- the complete authorization matrix, safe projections, CSRF/idempotency/version behavior, audit/log privacy, and no-store policy pass;
- parent and child flows meet the required error-state, keyboard, screen-reader, reflow, zoom, motion, contrast, and target-size acceptance;
- OpenAPI, backend, frontend, PostgreSQL, Docker, browser journey, security scan, SBOM, and Phase 3–8 regressions all pass;
- validation evidence names the tested commit/image digests and contains no unresolved critical/high product, security, privacy, accessibility, concurrency, or data-integrity finding.

The roadmap gate must remain unchecked until evidence demonstrates every condition. Phase 10 production launch remains blocked on both the Phase 8 and Phase 9 gates.
