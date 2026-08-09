# Phase 0 Decision Record

Status: **Approved by product owner on 2026-08-07**

## D-001 Product identity

- Working name: **Habit Home**
- Tagline: **Good habit starts from home**
- Tone: warm, calm, encouraging, child-friendly without appearing infantile.
- Visual direction: rounded cards, friendly animal avatars, bright-but-soft accessible colors, restrained motion.
- Avoid leaderboards, shame, punitive red, failure language, and point-loss framing.
- Starter avatars: fox, bear, rabbit, owl, cat, elephant, panda, and koala.

The name remains provisional until the product owner approves it and domain/trademark checks happen near launch.

## D-002 Timezone and week start

- Store one IANA timezone per household.
- Suggest the browser timezone during onboarding but require confirmation.
- Initial deployment default: `Asia/Jakarta`.
- Week-start choices: Monday or Sunday; default Sunday.
- Week start affects calendar presentation only; recurrence stores explicit weekdays.
- Store timestamps in UTC and derive occurrence dates and “today” in the household timezone.
- A timezone change affects current/future generation and display decisions but never rewrites stored occurrence local dates.

## D-003 Parent session and Parent Mode

- A trusted-device account session may persist for up to 30 days.
- Privileged Parent Mode locks after 5, 15, or 30 minutes of inactivity; default 15 minutes.
- Switching to a child or the profile picker locks Parent Mode immediately.
- Unlock uses an optional separately configured 6-digit parent PIN, with the account password as fallback.
- If no parent PIN exists, the password is required.
- Locking and authorization are enforced server-side; hiding the UI is insufficient.
- Password and PIN attempts are rate-limited and audited without storing secrets.

## D-004 Points

- Assignment points are positive whole numbers from 1 through 10,000.
- Default value: 5; quick choices: 1, 2, 5, and 10.
- No child-facing negative points or punishment flow exists.
- Parent manual correction is additive-only in MVP and requires a reason.
- Approval reversal creates an exact negative system compensation tied to the original award.
- A reversed occurrence is terminal and cannot earn points again.

## D-005 Parent accounts

- MVP supports exactly one owner account per household.
- Invitations, second-parent login, and email recovery are out of scope.
- Membership and actor identifiers remain plural-ready in the schema for later expansion.
- No dormant invitation UI or API is exposed.
- A parent can view each child's progress report on a daily, weekly, and monthly basis.

## D-006 Child identity boundary

- Child PINs remain optional.
- Without a PIN, profile selection is a trusted-device convenience, not identity assurance.
- Families requiring sibling isolation must enable child PINs.
- Backend authorization always scopes a child session to one child, regardless of PIN configuration.

## D-007 Approval reversal

- `approval_reversed` is a terminal occurrence state.
- Reversal requires a reason and creates a compensating ledger entry in the same transaction.
- The original award and all audit history remain immutable.
- If work must be assigned again, a parent creates a replacement one-off task.

## D-008 Remaining occurrence policies

- Recurring habits are actionable only on their occurrence date.
- One-off tasks remain actionable after their due date until completed or cancelled.
- Pending work for an archived child remains parent-reviewable; the child cannot make new submissions.
- Rejection reason is optional and written in child-safe language.
- Rejection or withdrawal returns an occurrence to `not_started` and permits a new attempt.
- Parents cannot submit completion on behalf of a child in MVP.
- Future unstarted occurrences may be replaced by effective-date edits; pending and decided occurrences never change.
- Only approved occurrences appear under Child Today **Done**.

## Approval record

D-001 through D-008 were approved by the product owner on 2026-08-07. Later changes must update this record, the requirements, and the roadmap together.
