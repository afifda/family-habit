# Family Habit Tracker Roadmap

This checklist is the execution order for the MVP. Items marked **Gate** block later phases. Every completed implementation item must include relevant tests and documentation.

## Progress

- [x] Phase 0 — Product and UX decisions
- [x] Phase 1 — Repository and development foundation
- [x] Phase 2 — Data model and backend foundation
- [x] Phase 3 — Parent authentication and household
- [x] Phase 4 — Child profiles and secure switching
- [ ] Phase 5 — Habits, schedules, and one-off tasks
- [ ] Phase 6 — Child Today and completion workflow
- [ ] Phase 7 — Parent approval, points, and history
- [ ] Phase 8 — Product integration and quality
- [ ] Phase 9 — VPS production launch
- [ ] Phase 10 — Pilot and MVP release

## Phase 0 — Product and UX decisions

Goal: remove ambiguity before implementation.

- [x] Confirm product name and basic visual tone.
- [x] Confirm the household timezone and week-start setting.
- [x] Confirm parent session idle timeout and Parent Mode unlock behavior.
- [x] Confirm the 1–10,000 point range.
- [x] Confirm that second-parent invitations remain outside MVP.
- [x] Define the full occurrence state-transition table and ledger effect of each transition.
- [x] Create a sitemap for shared, child, and parent areas.
- [x] Create mobile wireframes for profile picker, Child Today, task detail, parent dashboard, approval queue, children, and assignment form.
- [x] Create desktop/tablet adaptations for parent screens.
- [x] Review the clickable core-flow prototype against PR-01 through PR-10.
- [x] **Gate:** product requirements and core wireframes are approved.

Deliverables: approved requirements, state table, sitemap, wireframes, and prototype.

## Phase 1 — Repository and development foundation

Goal: create a reproducible monorepo and automated quality baseline.

- [x] Scaffold `backend/`, `frontend/`, `deploy/`, and `docs/`.
- [x] Initialize the Go service and internal module boundaries.
- [x] Initialize React + TypeScript + Vite with routing and test infrastructure.
- [x] Add local Docker Compose with PostgreSQL and the API.
- [x] Add environment configuration examples without real secrets.
- [x] Add formatter, linter, type-check, test, and build commands.
- [x] Add contribution/development instructions to the root README.
- [x] **Gate:** a new developer can start the documented stack and all local foundation checks pass.

Dependencies: Phase 0 gate.

## Phase 2 — Data model and backend foundation

Goal: establish durable household, scheduling, completion, and ledger invariants.

- [x] Write the initial OpenAPI contract and shared error format.
- [x] Design and review the entity relationship diagram.
- [x] Add migrations for users, families, memberships, and sessions.
- [x] Add migrations for children, habits, assignments, schedules, and one-off tasks.
- [x] Add migrations for occurrences, completions, point ledger, and audit events.
- [x] Add household-scope and uniqueness constraints.
- [x] Configure `pgx` pooling and type-safe SQL query generation.
- [x] Implement UTC timestamp and household-local-date utilities with DST tests.
- [x] Implement structured logging, request IDs, graceful shutdown, liveness, and readiness.
- [x] Add migration and database integration test harness.
- [x] **Gate:** migrations apply from empty, invariants are tested, and health checks pass.

Dependencies: Phase 1 gate.

## Phase 3 — Parent authentication and household

Goal: provide secure parent access and household configuration.

- [x] Implement parent registration, login, logout, and current-session endpoints.
- [x] Implement Argon2id password hashing and secure cookie sessions.
- [x] Add CSRF protection, session expiry/revocation, and login rate limiting.
- [x] Implement household onboarding with name and IANA timezone.
- [x] Implement backend parent authorization middleware and service checks.
- [x] Build registration, login, onboarding, and Parent Mode layouts.
- [x] Add authentication, authorization, validation, and session tests.
- [x] **Gate:** a parent can securely create and re-enter a household; unauthorized requests fail.

Requirements: PR-01 and the authentication sections of technical requirements.

## Phase 4 — Child profiles and secure switching

Goal: let families manage children and safely use a shared device.

- [x] Implement child list, create, update, and archive endpoints.
- [x] Implement unique active nickname validation and preserved history.
- [x] Implement optional child PIN hashing and rate-limited verification.
- [x] Implement restricted child-session creation and termination.
- [x] Enforce active-child, wrong-child, and cross-household authorization rules.
- [x] Build parent child-list/editor screens.
- [x] Build accessible household profile picker and active-profile indicator.
- [x] Lock Parent Mode on profile switch and inactivity.
- [x] Add role/authorization matrix integration tests.
- [x] **Gate:** a child can enter only their profile and cannot perform any parent operation.

Requirements: PR-02 and PR-03.

## Phase 5 — Habits, schedules, and one-off tasks

Goal: let parents define the work that appears on children's days.

- [x] Implement habit CRUD/deactivation.
- [x] Implement child assignment and multi-child occurrence separation.
- [x] Implement daily and selected-weekday schedules with effective dates.
- [x] Implement one-off task CRUD/cancellation and overdue behavior.
- [x] Implement deterministic lazy occurrence generation.
- [x] Snapshot title and points on dated occurrences.
- [x] Prevent duplicate occurrences through constraints and concurrency tests.
- [x] Build habit/task list and progressive create/edit forms.
- [x] Add explicit “this and future” behavior for recurring edits.
- [x] Test weekday, timezone, DST, edit, deactivation, and overdue cases.
- [x] **Gate:** expected occurrences are correct and history cannot be changed by editing a definition.

Requirements: PR-04 and PR-05.

## Phase 6 — Child Today and completion workflow

Goal: deliver the first complete child journey.

- [x] Implement child day endpoint using household-local dates.
- [x] Implement idempotent completion submission.
- [x] Implement withdrawal while pending.
- [x] Build To do, Waiting for parent, and Done groups.
- [x] Build task/habit detail and **I did it** interaction.
- [x] Add accessible live feedback and duplicate-tap protection.
- [x] Implement loading, empty, error, permission, and retry states.
- [x] Add component, accessibility, API, and concurrency tests.
- [x] **Gate:** a child can submit and withdraw work without gaining points or escaping their permissions.

Requirements: PR-06 and child portions of PR-07.

## Phase 7 — Parent approval, points, and history

Goal: complete the trusted points workflow.

- [x] Implement pending-approval query.
- [x] Implement transactional, idempotent approval and point award.
- [x] Implement rejection and return to `not_started`.
- [x] Implement approval reversal with a compensating ledger entry.
- [x] Implement audited parent point correction without a punishment UI.
- [x] Implement child balance, recent ledger, and 30-day history endpoints.
- [x] Implement per-child daily, weekly, and monthly reporting endpoints using household-local boundaries.
- [x] Build parent approval queue and decision interactions.
- [x] Build child points/activity and parent child-history screens.
- [x] Add concurrency tests proving exactly-once point awards.
- [x] Add audit-history and cross-household privacy tests.
- [x] **Gate:** every point is traceable and duplicate awards are impossible under concurrent requests.

Requirements: PR-07 and PR-08.

## Phase 8 — Product integration and quality

Goal: finish the household experience and meet the release quality bar.

- [x] Build parent overview with per-child progress and pending counts.
- [x] Build daily, weekly, and monthly per-child report views.
- [x] Apply the design system and responsive layouts across all screens.
- [ ] Complete keyboard, focus, contrast, screen-reader, text scaling, and reduced-motion review.
- [x] Verify all loading, empty, error, and permission states.
- [ ] Add end-to-end tests for the complete parent-to-child-to-parent journey.
- [x] Add security headers, request limits, and safe-log review.
- [x] Run dependency and container vulnerability scans and resolve release blockers.
- [ ] Test core journeys on representative phone, tablet, and desktop sizes.
- [x] Map PR-01 through PR-10 to passing automated or documented manual tests.
- [ ] **Gate:** all MVP acceptance criteria pass and no critical/high security issue remains open.

Requirements: PR-09, PR-10, and all non-functional requirements.

## Phase 9 — VPS production launch

Goal: deploy a secure, observable, and recoverable production system.

- [ ] Provision VPS, low-privilege deploy user, SSH keys, firewall, Docker, and Compose.
- [ ] Configure domain DNS and Caddy automatic HTTPS.
- [ ] Create pinned multi-stage production images running as non-root where possible.
- [ ] Create production Compose configuration with private API/database networking.
- [ ] Define and securely provision production secrets.
- [ ] Add explicit migration and controlled deployment workflow.
- [ ] Configure external uptime monitoring and operational alerts.
- [ ] Configure encrypted nightly `pg_dump` backups to offsite storage.
- [ ] Document deploy, rollback, backup, restore, and incident procedures.
- [ ] Restore a production-shaped backup into a temporary database and verify it.
- [ ] **Gate:** HTTPS, monitoring, backups, and tested restoration are working.

Dependencies: Phase 8 gate and production prerequisites in the technical requirements.

## Phase 10 — Pilot and MVP release

Goal: verify usefulness and safety with the family before expanding scope.

- [ ] Create the real household and initial child profiles.
- [ ] Add a small initial set of habits rather than migrating every routine at once.
- [ ] Run scripted authorization and duplicate-award checks in production.
- [ ] Observe a child completing a task without help and record usability issues.
- [ ] Measure setup time, completion time, submission success, and approval turnaround.
- [ ] Triage pilot findings into defects, required refinements, and post-MVP ideas.
- [ ] Complete the four-week pilot success-criteria review.
- [ ] **Release gate:** success criteria are reviewed, critical defects are closed, and recovery remains tested.

## Post-MVP candidates

These are deliberately not part of the checklist above:

- Second-parent invitations and account recovery
- Rewards catalog and redemption
- Streaks and comparative/gamified summaries beyond the approved progress reports
- Reminders and notifications
- PWA installation and limited offline behavior
- Data export and deletion workflows
- Additional recurrence patterns
- Multiple households per parent
- Managed PostgreSQL or separate object storage if scale requires it

## Deferred engineering work

- Enable and validate the prepared CI workflow when a remote repository and CI policy are ready.
- Add migration execution to CI after Phase 2 introduces the migration harness.

## Definition of done for roadmap items

A roadmap implementation item can be checked only when:

- [ ] Code and migrations are implemented where applicable.
- [ ] Acceptance criteria are satisfied.
- [ ] Relevant unit, integration, component, or end-to-end tests pass.
- [ ] Authorization and household isolation were considered.
- [ ] Accessibility states were considered for visible UI changes.
- [ ] API and operational documentation is updated.
- [ ] No known release-blocking regression was introduced.
