# Phase 4 validation

Validated on 2026-08-09 against PostgreSQL 16.6 and the local Docker stack.

## Delivered behavior

- Parents can list, create, edit, and archive child profiles. Archival is idempotent, retains history, removes the profile from the picker, and downgrades active sessions for that child.
- Active nicknames are trimmed and unique case-insensitively within a household. Avatar and color values are required and constrained by both the API and database.
- Child PINs are optional 4–6 digit values stored only as Argon2id hashes. Update distinguishes preserve, replace, and remove operations.
- `/api/v1/profiles` exposes only ID, nickname, avatar, color, and `pinRequired`; parent administration data remains under parent-only `/api/v1/children`.
- Entering child mode validates the active household child and optional PIN atomically, clears Parent Mode, and records the exact active child. Leaving child mode is idempotent.
- Invalid, unknown, archived, cross-household, missing-PIN, and wrong-PIN entry attempts use the same generic response. Rate limiting covers session/child and source-wide buckets and audits only hashed identifiers.
- Parent mutations re-check the persisted active owner session and Parent Mode inside their transactions, preventing a concurrent profile switch or idle lock from retaining privilege.
- The frontend provides real profile loading, empty, error/retry and PIN states, an active nickname/avatar indicator, role guards, parent child editing, PIN controls, and an accessible archive dialog.

## Review and corrections

The independent Phase 4 security review initially blocked release for profile/PIN oracle responses, shared projection overexposure, raceable parent authorization, incomplete limiter boundaries, non-idempotent archive/leave behavior, missing archived-session repair, weak database normalization, unclear active identity, and absent Phase 4 tests. Each blocker was corrected before the gate was accepted.

The clean integration run also found a stale Phase 2 test fixture that omitted the newly required avatar and color. The fixture was updated and the entire suite rerun successfully.

## Gate evidence

- Applied migrations `000001` through `000007` to a newly created empty `family_habit_phase4_test` database.
- Regenerated sqlc models with migration 7 included; the development database reports versions 1–7.
- Backend `go test ./...`, `go vet ./...`, and API/migration builds passed.
- PostgreSQL integration suites passed for schema, authentication, child lifecycle, nickname reuse after archive, authorization, PIN privacy, archival downgrade, and HTTP projection behavior.
- Frontend Prettier, ESLint with zero warnings, TypeScript, 18 Vitest tests across 5 files, and the Vite production build passed.
- `git diff --check` passed and the OpenAPI contract includes separate parent and picker projections.
- The API, PostgreSQL, and frontend containers rebuilt successfully and reached healthy state.
- Live shared-device flow passed: created PIN-free and protected children; verified the limited profile projection; entered both profiles; confirmed child mode receives `403` from parent `/children`; received the same generic `401 child_profile_unavailable` for a wrong PIN; left child mode; unlocked Parent Mode; archived twice with `204`; confirmed the archived profile disappeared from the picker but remained in the parent archive list.

## Deployment note

The PIN and login limiters remain process-local for the current single API instance on one VPS. Move them to shared persistence before adding API replicas.

The disposable `family_habit_phase4_test` database is isolated from the development database and may be removed at any time.
