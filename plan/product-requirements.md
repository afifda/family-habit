---
name: family-habit-tracker
description: Product requirements for a child-friendly, parent-managed family habit tracker
status: backlog
created: 2026-08-07T09:32:53Z
---

# Product Requirements: Family Habit Tracker

## Executive summary

Build a private, responsive family habit tracker in which parents manage children, recurring habits, one-off tasks, schedules, approvals, and points. Children use a simplified shared-device experience to select their profile, see what they need to do today, and submit completed work. Points are awarded exactly once after parent approval.

## Problem statement

Generic task managers are too complex for young children and do not provide the combination of easy profile switching, parent-controlled assignments, approval, and motivating points that a family needs. The product must make the daily child interaction fast while keeping administrative and point-changing actions protected.

## Goals

- Let a parent create a household, child, and first recurring habit in under 10 minutes.
- Let a child find and submit a scheduled task in under 10 seconds.
- Give parents a trustworthy approval workflow and auditable point history.
- Work well on shared phones, tablets, and desktop browsers.
- Keep household and child data private and recoverable on a self-hosted VPS.

## Users and permissions

### Parent

A parent can manage the household, children, assignments, schedules, approvals, and point corrections. Parent Mode requires authenticated access and must lock when switching to a child profile.

### Child

A child can access only their own Today view, progress, and recent point activity. A child can submit or withdraw a pending completion but cannot approve work, edit assignments, change points, or enter Parent Mode.

### Household

A household is the data and authorization boundary. Every parent, child, habit, occurrence, completion, and ledger entry belongs to exactly one household.

## Core terminology

- **Habit:** a recurring assignment scheduled daily or on selected weekdays.
- **Task:** a one-off assignment with a due date.
- **Occurrence:** the dated instance of a habit or task shown to a child.
- **Completion:** a child's claim that an occurrence is done.
- **Approval:** a parent's decision that awards points.
- **Point ledger:** the append-only history of point awards and corrections.

## User stories and acceptance criteria

### PR-01 Parent authentication and household setup

As a parent, I can create and securely enter my household.

- [ ] A parent can register with email and password and then sign in and out.
- [ ] On first use, the parent sets a household name, confirms an IANA timezone, and chooses Monday or Sunday as the week start.
- [ ] The initial deployment suggests `Asia/Jakarta` and Sunday as defaults, while allowing the parent to change both during onboarding.
- [ ] Parent-only pages reject child sessions at the server, not only in the UI.
- [ ] Parent Mode locks after switching profiles and after a configurable inactivity period of 5, 15, or 30 minutes; the default is 15 minutes.
- [ ] The MVP supports one owner account; a second-parent invitation flow is deferred.

### PR-02 Child management

As a parent, I can maintain the list of children in my household.

- [ ] A parent can add and edit a child's nickname, avatar, and color.
- [ ] Nicknames must be unique among active children in a household.
- [ ] A parent can configure an optional 4–6 digit child PIN.
- [ ] A child can be archived but cannot be hard-deleted when history exists.
- [ ] Archived children disappear from the normal profile picker while history remains available to parents.

### PR-03 Profile switching

As a child, I can quickly open my own space on a shared device.

- [ ] The household profile picker uses large, labelled avatar cards.
- [ ] A child profile can be entered within two interactions when no child PIN is configured.
- [ ] The active profile is always visually and semantically clear.
- [ ] Switching from Parent Mode immediately removes parent privileges.
- [ ] Entering Parent Mode requires the parent password or a separately configured parent unlock credential.
- [ ] Without a child PIN, child profile selection is a trusted-device convenience and not strong identity verification.

### PR-04 Recurring habit management

As a parent, I can assign recurring habits to one or more children.

- [ ] A habit has a title, optional description/icon/color, point value, schedule, and effective start date.
- [ ] Schedules support every day or selected weekdays in the household timezone.
- [ ] One habit can be assigned to multiple children, producing independent occurrences.
- [ ] Points are positive whole numbers between 1 and 10,000.
- [ ] Editing a habit changes only current and future occurrences after a clearly selected effective date.
- [ ] Deactivating a habit prevents future occurrences without deleting history.

### PR-05 One-off task management

As a parent, I can assign a dated one-off task to a child.

- [ ] A task has a title, child, due date, and positive point value.
- [ ] The task appears on the assigned child's due date.
- [ ] An incomplete overdue task remains actionable until completed or cancelled.
- [ ] A task can award points no more than once.

### PR-06 Child Today experience

As a child, I can understand what I need to do today.

- [ ] Items are grouped into **To do**, **Waiting for parent**, and **Done**.
- [ ] Each item shows its title, point value, type, and meaningful due-state label.
- [ ] The primary action is labelled **I did it**, not represented by an ambiguous checkbox.
- [ ] A child can submit an item once and receives immediate, accessible feedback.
- [ ] Empty, loading, failed, and offline/retry states are designed and implemented.
- [ ] Overdue tasks are described neutrally and are not presented as punishment.

### PR-07 Completion and approval workflow

As a child, I can submit work, and as a parent, I can review it.

- [ ] A submitted item becomes `pending_approval` and does not award points yet.
- [ ] A child can withdraw their submission only while it is pending.
- [ ] The parent queue displays child, item, submission time, and proposed points.
- [ ] Approval records the parent and time and awards points exactly once.
- [ ] Rejection returns the occurrence to `not_started` and records the decision.
- [ ] Repeated requests and concurrent clicks cannot duplicate completions or point awards.
- [ ] Every approval, rejection, withdrawal, and correction is auditable.

### PR-08 Points and history

As a family member, I can understand earned points and recent progress.

- [ ] A child can see their current point balance and recent entries.
- [ ] A parent can see each child's balance and at least 30 days of occurrence history.
- [ ] Siblings cannot see another child's detailed progress by default.
- [ ] Parents can issue an audited additive correction; negative punishment actions are not offered.
- [ ] Reversing an approval creates a compensating ledger entry and never erases history.
- [ ] A reversed approval is terminal for that occurrence; it cannot be submitted or awarded again.

### PR-09 Parent overview

As a parent, I can quickly understand today's household activity.

- [ ] The dashboard shows active children, completed/total items, and pending approvals.
- [ ] Pending approvals link directly to the review queue.
- [ ] The dashboard uses the household timezone for all daily totals.
- [ ] A parent can view per-child daily, weekly, and monthly reports.
- [ ] Reports show assigned, submitted, approved, rejected, and incomplete occurrence counts plus points earned for the selected period.
- [ ] Weekly report boundaries follow the household week-start setting; monthly reports follow household-local calendar months.

### PR-10 Responsive and accessible experience

As a user, I can operate the site on common devices and with assistive technology.

- [ ] The child flow is mobile-first and works on phone, tablet, and desktop widths.
- [ ] Interactive targets are at least 48 by 48 CSS pixels where practical.
- [ ] Icons and colors never replace text labels; focus and status are programmatically available.
- [ ] Keyboard navigation, visible focus, text scaling, and reduced motion are supported.
- [ ] The release targets WCAG 2.2 AA for core journeys.

## Non-functional product requirements

- Child data collection is limited to nickname, avatar/color, optional PIN verifier, and application activity.
- Birth dates, photos, addresses, school details, and public profiles are not required.
- Destructive or point-changing parent actions require clear confirmation where appropriate.
- Routine child completion must not require unnecessary confirmation dialogs.
- Product language should encourage personal progress and avoid public sibling rankings or shame-based messaging.

## Success criteria

During a four-week pilot:

- At least 80% of test children can submit a task without assistance.
- A parent can add a child and recurring habit in under 3 minutes after account setup.
- At least 80% of scheduled occurrences receive a completion decision.
- At least 95% of completion actions synchronize on their first attempt.
- Zero duplicate point awards occur.
- Zero scripted child-to-parent privilege escalation tests succeed.
- Backup restoration succeeds before the production pilot begins.

## Out of scope for MVP

- Native mobile applications and push notifications
- Offline-first synchronization
- Rewards catalog and redemption
- Negative points or punishment workflows
- Streaks, badges, leaderboards, and public sibling comparisons
- Photo/video proof, chat, and social features
- Monthly or arbitrary interval recurrence
- Calendar integrations and task dependencies
- Multiple households per parent
- Parent invitations and password-reset email delivery
- Localization beyond preparing the UI for translatable strings

## Assumptions and constraints

- The initial deployment serves one family but must preserve household isolation in the data model and authorization layer.
- Parents and children have reliable internet access while using the application.
- Children primarily use an already authenticated, trusted household device.
- The application is hosted on one Linux VPS and therefore has one runtime failure domain.
- PostgreSQL is the authoritative database.

## Product dependencies

- Final application name, visual tone, and preferred starter avatars
- Household timezone and week-start preference
- Domain name and DNS access before production deployment
- Decision on parent inactivity timeout before authentication implementation
