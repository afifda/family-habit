# Technical Requirements

## Architecture

The MVP is a modular monolith deployed with Docker Compose:

```text
Browser → HTTPS/Caddy → React static application
                    └→ Go REST API → PostgreSQL
```

- Frontend: React, TypeScript, Vite, React Router, TanStack Query, React Hook Form, and Zod.
- Backend: Go, `chi`, `pgx`, `sqlc`, and versioned database migrations.
- Database: PostgreSQL 16 or newer.
- Contract: versioned REST endpoints under `/api/v1` described by OpenAPI.
- Infrastructure: Caddy, Go API, PostgreSQL, and optional backup container under Docker Compose.
- Real-time transports and microservices are not required for MVP.

## Repository layout

```text
backend/
  cmd/api/
  internal/{auth,family,children,habits,completions,points}/
  migrations/
  openapi/
frontend/
  src/{api,components,features,routes}/
deploy/
  compose.yaml
  Caddyfile
docs/
plan/
```

## Data requirements

### Minimum entities

- `users`: parent email, Argon2id password hash, status, timestamps.
- `families`: name, IANA timezone, settings.
- `family_memberships`: user, family, and role.
- `children`: family, nickname, avatar/color, optional PIN hash, active status.
- `habits`: reusable household-owned definition.
- `habit_assignments`: habit-to-child assignment and effective dates.
- `habit_schedules`: daily or selected-weekday schedule.
- `occurrences`: dated assignment with title and point snapshots.
- `completion_attempts`: append-only child submissions and parent decision metadata; rejected or withdrawn occurrences may have later attempts.
- `point_ledger`: immutable credits and compensating corrections.
- `audit_events`: security- and point-relevant actions.

### Data invariants

- All identifiers exposed externally are non-sequential UUIDs.
- All timestamps are stored in UTC; local dates use the family's IANA timezone.
- A unique constraint prevents duplicate occurrences for an assignment, child, and local date.
- A unique constraint prevents more than one point award for an occurrence; reversal creates one compensating entry and makes the occurrence terminal.
- Approval and point-ledger insertion occur in one database transaction.
- Occurrences snapshot title and point value to preserve history.
- Balances are calculated from the ledger or a transactionally maintained projection.
- Historical data is archived or compensated, never silently rewritten.
- Every query that accesses household data is scoped by household ID.

## API requirements

The initial OpenAPI contract must cover:

- Authentication: register, login, logout, current session.
- Household: read and update household settings.
- Children: list, create, update, archive.
- Child sessions: enter child profile, leave child profile.
- Habits: list, create, update, deactivate.
- Assignments/schedules: create, update from effective date, deactivate.
- Tasks: create, update, cancel.
- Today: retrieve occurrences for a child and local date.
- Completion: submit and withdraw.
- Review: list pending, approve, reject, reverse approval.
- Points/history: balance, ledger, occurrence history, parent correction.
- Reports: per-child daily, weekly, and monthly aggregate progress using household-local date boundaries.
- Operations: liveness and readiness endpoints.

Mutating endpoints must return structured validation errors and use idempotency or database constraints where duplicate submission would cause harm.

## Authentication and authorization

- Parent passwords use Argon2id with parameters documented in code.
- Authentication uses secure, HTTP-only, `SameSite=Lax` cookies; tokens are not stored in browser local storage.
- State-changing requests use CSRF protection appropriate to the cookie/session design.
- Child sessions are restricted to one child and contain no parent privileges.
- Parent routes and operations enforce authorization in backend middleware and service logic.
- Entering Parent Mode requires parent authentication; leaving it invalidates privileged session state.
- Login and PIN attempts are rate-limited and audited without logging secrets.
- PINs are stored only as slow hashes and are never treated as equivalent to parent authentication.
- Sessions can be revoked and have defined idle and absolute expiry.

## Security and privacy

- Only Caddy ports 80 and 443 are public; API and PostgreSQL remain on a private container network.
- Containers run as non-root where their images permit it.
- Secrets come from uncommitted production environment/secret files.
- SQL queries are parameterized and request bodies have size limits.
- Responses include appropriate CSP, HSTS, content-type, framing, and referrer headers.
- Logs exclude passwords, PINs, cookies, tokens, and child notes.
- Dependency and container image vulnerability scans run in CI.
- Authorization tests cover every parent/child operation and cross-household attempts.

## Frontend requirements

- Routes and navigation are role-aware, but backend authorization remains authoritative.
- Server state uses a consistent query/cache layer; mutations prevent duplicate submission.
- Forms share validation rules with the documented API contract where possible.
- Core screens implement loading, empty, validation, permission, retry, and success states.
- Components meet the accessibility criteria in PR-10.
- Layouts are mobile-first with tablet and desktop adaptations.
- Dates are displayed using the household timezone and include explicit dates for overdue or boundary-sensitive items.
- Report queries use the household timezone and configured week start, with aggregate counts derived from occurrences rather than mutable cached client data.
- The UI is prepared for localization by centralizing user-facing strings.

## Backend requirements

- Internal packages separate auth, household, child, scheduling, completion, and points concerns.
- Business rules reside outside HTTP handlers and are unit testable.
- Database access uses generated or otherwise type-safe queries.
- Migrations are forward-versioned and run as an explicit deployment step.
- Structured logs include request IDs and safe actor/household identifiers.
- Graceful shutdown closes HTTP and database resources.
- `/health/live` checks the process; `/health/ready` verifies required dependencies.

## Testing requirements

### Backend

- Unit tests: status transitions, authorization, schedules, timezone/DST boundaries, and ledger rules.
- Unit tests: reporting day/week/month boundaries, including Sunday week start, month changes, timezone changes, and DST boundaries.
- PostgreSQL integration tests: migrations, constraints, approval transactions, and repository queries.
- Concurrency/idempotency tests prove an occurrence cannot award points twice.
- Authorization matrix tests cover parent, active child, wrong child, unauthenticated, and cross-household access.

### Frontend

- Component tests cover profile selection, Today groups, forms, review decisions, and error states.
- Automated accessibility checks cover core screens.
- Playwright tests cover onboarding, child creation, habit creation, child submission, parent approval, and point display.

### Continuous integration

- Go formatting, static analysis, tests, and build.
- TypeScript formatting/linting, type checking, tests, and production build.
- Database migration test from an empty schema.
- Dependency and image scanning before release.

## Deployment and operations

- Production uses pinned container image versions and restart/health policies.
- CI builds immutable images; production does not build from a mutable working tree.
- Deployment runs migrations once, then starts or replaces the API and frontend.
- PostgreSQL data uses a named persistent volume.
- External monitoring checks HTTPS availability.
- Alerts cover downtime, repeated 5xx responses, high disk usage, backup failure, and certificate failure.
- A documented rollback procedure must distinguish application rollback from schema rollback.

## Backup and recovery

- Run encrypted nightly logical PostgreSQL backups.
- Copy backups to storage outside the VPS; a local-only backup is insufficient.
- Retain at least 7 daily, 4 weekly, and 6 monthly backups.
- Back up deployment configuration and required encryption material separately.
- Test restoration into a temporary database before production launch and monthly afterward.
- Document the restore procedure and expected recovery time.

## Performance and reliability targets

- Core API reads should have a 95th-percentile server response time below 500 ms at household-scale load, excluding network latency.
- Core mutation endpoints should have a 95th-percentile response time below 750 ms.
- Normal family waking-hour availability target is 99% during the pilot.
- The application must recover automatically from a process restart without data loss.
- Duplicate point awards are a release-blocking defect.

## Production prerequisites

- Linux VPS with at least 2 vCPU, 4 GB RAM, and adequate persistent disk.
- Domain name and DNS access.
- SSH key access and a low-privilege deployment account.
- Firewall permitting only SSH, HTTP, and HTTPS publicly.
- Offsite S3-compatible backup destination and encryption credentials.
- External uptime monitor.
