# Information Architecture and Journeys

## Sitemap

```text
Habit Home
├── Shared device
│   ├── Profile picker
│   ├── Optional child PIN entry
│   ├── Parent Mode unlock
│   └── Session and error states
├── Child area
│   ├── Today
│   │   ├── To do
│   │   ├── Waiting for parent
│   │   └── Done
│   ├── Task detail
│   ├── My points
│   ├── Recent activity
│   └── Switch profile
└── Parent area
    ├── Overview
    ├── Approvals
    │   ├── Pending queue
    │   └── Review detail
    ├── Children
    │   ├── Child list
    │   ├── Add/edit child
    │   ├── Child history
    │   └── Child reports
    │       ├── Daily
    │       ├── Weekly
    │       └── Monthly
    ├── Habits and tasks
    │   ├── Active assignments
    │   ├── Add/edit recurring habit
    │   ├── Add/edit one-off task
    │   └── Archived/cancelled
    ├── Points
    │   ├── Balances
    │   ├── Ledger
    │   └── Additive correction
    ├── Household settings
    └── Exit Parent Mode
```

## Navigation

- Child navigation: **Today**, **My points**, and a persistent avatar/profile-switch control.
- Mobile parent navigation: **Home**, **Approvals**, **Tasks**, and **More**.
- Tablet/desktop parent navigation: persistent sidebar with Overview, Approvals, Habits, Children, Points, Settings, and Exit Parent Mode.
- Leaving Parent Mode replaces privileged browser history so Back cannot restore an unlocked page.

## Journey J-01 Parent onboarding

```text
Enter account details → Enter household name and confirm timezone/week start
→ Submit registration atomically → Parent Mode
→ Add first child → Choose avatar/color and optional PIN
→ Create first recurring habit → Review → Save → Parent overview
```

The first two screens are client steps over one registration submission. The API atomically creates the owner, household, owner membership, and authenticated Parent Mode session; it does not retain a partially onboarded account.

## Journey J-02 Child completion

```text
Profile picker → Select child → Optional PIN → Today
→ Open item → I did it → Waiting for parent
```

No confirmation dialog appears for routine submission. The system announces success and prevents duplicate taps.

## Journey J-03 Child withdrawal

```text
Today → Waiting for parent → Open item
→ Undo — I’m not finished → Item returns to To do
```

Withdrawal is unavailable after a parent decision.

## Journey J-04 Parent review

```text
Parent Mode → Unlock → Approvals → Review item
├── Approve · +N points → Award once → Next item
└── Not yet → Optional child-safe reason → Child To do
```

## Journey J-05 Profile security

```text
Parent Mode → Exit/Switch → Server locks privileges
→ Profile picker → Select child
```

Browser Back must not restore privileged content.

## Journey J-06 Recurring habit creation and edit

```text
Habits and tasks → New recurring habit → Basics
→ Children → Schedule → Points → Review → Save
```

Editing requires an effective date and clearly states that past history will not change.

## Journey J-07 Parent reporting

```text
Parent overview → Select child → Reports
→ Choose Day / Week / Month → Navigate period
→ Review assigned, submitted, approved, rejected, incomplete, and points totals
```

Day boundaries use the household timezone, week boundaries use the configured Sunday start, and months use household-local calendar months.
