# Routine Groups and Reward Redemption Plan

Status: approved for immediate Phase 9 implementation
Date: 2026-08-18

## Existing capability audit

The product does not currently have configurable routine groups. Child Today groups occurrences only by workflow state: **To do**, **Waiting for parent**, and **Done**. Habits and assignments have no routine/group reference, and one-off tasks have no grouping field.

Reward redemption also does not exist. It is explicitly listed as post-MVP scope. The point ledger currently supports awards, approval reversals, and positive parent corrections; children cannot create point-changing entries.

## Feature A: dynamic routine groups

### Product model

A routine group is an optional household-defined way to organize a child's work, such as Morning, After school, Evening, Weekend, or Chores. It controls presentation and ordering only. It does not own recurrence, due dates, completion state, or points.

Parents can:

- create, rename, reorder, color, icon, and archive groups;
- optionally define a display time window for context, not enforcement;
- assign a habit assignment or one-off task to a group;
- use the same group for several children or assign the same habit to different groups for different children.

Children see household-local Today items ordered by group and then by item order. Items without a group appear in a virtual **Other** section. Workflow status remains visible on each item and as filters/counts; it is not replaced by the routine group.

Recommended starter groups are Morning, Afternoon, and Evening, offered during setup but never hard-coded. A household may start empty and create any names it wants.

### Data model

Add `routine_groups`:

- `id`, `family_id`, `name`, `icon`, `color`;
- nullable `starts_at_local` and `ends_at_local` for display hints;
- `sort_order`, `archived_at`, `version`, timestamps;
- active name uniqueness per household using `lower(btrim(name))`;
- structural household-scoped keys.

Add nullable `routine_group_id` and `sort_order` to `habit_assignments` and `tasks`. The assignment is the correct level for recurring work because one habit may belong to different groups for different children. Existing occurrence snapshots should add nullable group ID/name/icon/color/order snapshots so history never changes after a group rename or reassignment.

Group archival does not delete or rewrite historical occurrences. Current/future assignments become ungrouped or must be reassigned in the same parent transaction. Recommend requiring the parent to choose **Move to Other** or another active group when archiving a group that is in use.

### API and UI

- Parent CRUD: `/api/v1/routine-groups` with idempotency and `If-Match` on mutations.
- Reorder endpoint accepts the complete ordered active-ID list in one transaction.
- Habit assignment and task create/update accept nullable `routineGroupId` and `sortOrder`.
- Today/history responses expose a small immutable `routineGroup` projection and server-computed ordering.
- Parent **Habits & Tasks** gains group management and drag/move controls with accessible move-up/down alternatives.
- Child Today defaults to routine sections; status filters expose All, To do, Waiting, and Done.

### Important rules

- Groups are optional and presentation-only.
- All date/time hints use the household timezone.
- No routine group grants authorization or changes completion transitions.
- Archived children and cross-household IDs remain inaccessible.
- Reordering does not rewrite occurrence history.

## Feature B: optional reward catalog and redemption

### Product model

The household has a parent-controlled `rewards_enabled` setting, off by default. When enabled, parents define rewards with a title, optional description/icon, positive integer point cost, availability, and optional child assignments.

Recommended redemption lifecycle:

1. Child chooses an active reward and confirms the exact `-N points` effect.
2. The server atomically verifies the reward, child, feature flag, and sufficient balance.
3. It creates a redemption in `requested` state and immediately creates an immutable negative ledger entry. This reserves the points and prevents double spending.
4. Parent marks the request `fulfilled`, or cancels it with a reason.
5. Cancellation creates one exact positive refund entry linked to the original redemption debit. No ledger row is edited or deleted.

Immediate reservation is safer than deducting only on fulfillment: two devices cannot spend the same balance, and the child sees an accurate available balance. Fulfillment should not change points because they were already reserved.

### Data model

Add `rewards`:

- `id`, `family_id`, title, description, icon, `cost_points`;
- active/effective dates, optional stock/limit fields deferred;
- `version`, archived timestamp, audit timestamps.

Add `reward_child_availability` only if per-child catalogs are needed; otherwise all active household children see the reward. Prefer adding this join table now because household-scoped availability is inexpensive and avoids later schema ambiguity.

Add `reward_redemptions`:

- household, child, reward, immutable reward title/cost snapshots;
- state `requested | fulfilled | cancelled`;
- requested/decided actor and timestamps, cancellation reason, version;
- one debit ledger link and optional refund ledger link.

Extend ledger kinds with `reward_redemption` and `reward_refund`. Enforce:

- redemption amount equals negative cost snapshot;
- one debit per redemption;
- refund exactly negates its debit and exists at most once;
- debit/refund, child, reward, actor, and household scope match structurally;
- ledger remains append-only.

### Concurrency and balance safety

Every redemption transaction must serialize on the child, either by locking the child balance row/projection or using a transaction-scoped advisory lock keyed by child ID. Under that lock, calculate the authoritative ledger balance, reject insufficient funds, create the redemption, ledger debit, audit event, and idempotency response in one transaction.

Never allow the balance to go below zero from redemption. Concurrent requests with the same key replay one result; different-key requests compete against the same locked balance. Cancellation/refund and fulfillment also lock the redemption and child in a fixed order.

### API and UI

- Parent reward catalog CRUD and enable/disable setting.
- Child catalog returns only eligible active rewards and `canRedeem` computed by the server.
- Child redeem mutation requires CSRF, `Idempotency-Key`, reward version, and explicit confirmation.
- Parent queue supports fulfill/cancel; cancellation shows the exact `+N points` refund.
- Child points activity uses neutral labels such as **Reward requested** and **Reward refunded**.
- Reports expose earned, redeemed, refunded, and net point changes separately.

### Privacy and motivation safeguards

- No sibling balances, catalogs, or redemption history are exposed.
- No negative custom deductions or parent punishment action is introduced.
- Parents define positive-cost rewards; children cannot edit costs or ledger entries.
- Avoid scarcity, streak, or leaderboard mechanics in the first version.

## Recommended delivery sequence

This plan is now **Phase 9**, promoted ahead of VPS production launch by product decision on 2026-08-18.

### Phase A — Routine foundation

- Schema, constraints, CRUD, reordering, assignment/task integration.
- Occurrence snapshots and Today server ordering.
- Parent manager and child routine-section UI.
- Migration, tenant, history, concurrency, accessibility, and timezone tests.

### Phase B — Reward foundation

- Household feature flag, catalog, availability, redemption state machine.
- Ledger extensions, structural constraints, balance serialization, audit/idempotency.
- Parent catalog/fulfillment UI and child catalog/redemption UI.
- Concurrency, insufficient-balance, refund, privacy, reporting, and accessibility tests.

### Phase C — Integrated release gate

- Browser journey: parent creates groups/rewards → assigns work → child earns points → redeems → parent fulfills/cancels → balances and reports reconcile.
- Migration from current production-shaped data proves existing items appear under Other and existing balances remain unchanged.
- Security and concurrency review proves no cross-household access, duplicate debit/refund, or negative redemption balance.

## Decisions to confirm before implementation

Recommended defaults are shown first:

1. Redemption reserves points immediately and parent later fulfills/cancels.
2. Rewards are available to selected children, with **all active children** as the form default.
3. Routine groups are household-defined and reusable, while membership is stored per child assignment/task.
4. Time windows are optional display hints and never make an item inaccessible.
5. Existing ungrouped items remain valid under a virtual **Other** section.
6. Rewards are disabled by default per household.

## Out of scope for the first release

- Reward inventory, expiration, recurring redemption limits, delivery integrations, wish lists, and child-authored rewards.
- Automatic routine transitions, notifications, timers, or time-based hiding.
- Nested groups, group dependencies, and group-level point bonuses.
- Any generic negative-point or punishment mechanism.
