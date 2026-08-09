# Phase 3 validation

Validated on 2026-08-08 against the local PostgreSQL 16.6 Docker stack.

## Delivered behavior

- Account and household registration commit atomically while the UI presents account and household settings as two accessible steps.
- Registration and login enter Parent Mode. The opaque account session lasts up to 30 days; Parent Mode locks independently after the household's 5, 15, or 30 minute idle setting.
- Passwords and parent PINs use versioned Argon2id hashes. Session and CSRF credentials are random values stored only as hashes.
- Cookies are HttpOnly, SameSite=Lax, scoped to `/`, and become Secure in production. Logout revokes the server session and clears the cookie.
- Mutations require a session-bound CSRF token. Parent routes enforce Parent Mode on the server.
- Authentication failures return generic credentials errors, use IP-and-identity rate limiting, and store only hashed identifiers in the authentication-attempt audit table.
- Household names, IANA timezones, Sunday/Monday week starts, idle settings, and optional six-digit parent PINs are validated. Household settings and PIN changes commit atomically.
- Request bodies are capped and API security headers are enabled.

## Gate evidence

- Applied migrations `000001` through `000006` to a newly created empty `family_habit_phase3_test` database.
- Regenerated sqlc models after all six migrations.
- Backend: `go test ./...`, `go vet ./...`, and builds for `cmd/api` and `cmd/migrate` passed.
- Database/auth integration tests passed against the disposable database, including atomic registration, hashed credential storage, CSRF verification, revocation, and schema constraints.
- Frontend: Prettier, ESLint with zero warnings, TypeScript, 9 Vitest tests, and the Vite production build passed.
- OpenAPI 3.1 YAML parsed and all 306 local references resolved.
- PostgreSQL, API, and frontend containers rebuilt and reached healthy state.
- Live lifecycle smoke passed: register returned Parent Mode; household read returned 200; lock returned the profile picker with a null idle expiry; password unlock returned Parent Mode; logout returned 204; the revoked session returned 401.

## Deployment note

The authentication limiter is intentionally process-local for the current single API instance on one VPS. It resets when the API restarts. Move it to shared persistence before running multiple API replicas.

The disposable `family_habit_phase3_test` database is isolated from the normal development database and may be removed at any time.
