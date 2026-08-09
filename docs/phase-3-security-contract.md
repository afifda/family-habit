# Phase 3 Authentication and Household Contract

This document resolves the implementation choices behind PR-01. The OpenAPI document remains the HTTP source of truth.

## Registration and onboarding

Registration is presented as two client steps: account details, followed by household name, confirmed IANA timezone, and Sunday-or-Monday week start. The client submits all fields once to `POST /api/v1/auth/register`. The API creates the user, family, owner membership, and initial session in one database transaction. Partial accounts or households are never retained after failure.

Registration does not use the authenticated idempotency store because no family or session exists yet. Normalized-email uniqueness plus the atomic transaction handles repeated submissions. The endpoint is source-rate-limited.

A successful registration or login rotates an opaque session credential and its CSRF credential and enters Parent Mode immediately. The account session has an absolute lifetime no longer than 30 days. Parent Mode has the household-configured 5, 15, or 30 minute idle lifetime and defaults to 15 minutes.

## Cookies, Parent Mode, and CSRF

The session cookie is named `habit_home_session`. It is opaque and uses `HttpOnly`, `SameSite=Lax`, `Path=/`, and a maximum age no longer than the session absolute lifetime. `Secure` is mandatory outside explicit local HTTP development; no `Domain` attribute is set. Only a hash of the bearer credential is persisted. Logout revokes the server-side session and expires the browser cookie.

Every authenticated state-changing request requires the session-bound token in `X-CSRF-Token`. Missing or invalid authentication returns `401 unauthorized`. A valid session with missing or invalid CSRF returns `403 csrf_invalid`. An authenticated actor without the required role, or a session whose Parent Mode has locked, returns `403 forbidden`.

The current-session response always includes `absoluteExpiresAt`. `idleExpiresAt` contains the active Parent Mode deadline and is `null` in profile-picker/shared or child mode. The server, not the browser, enforces idle and absolute expiry. Locking Parent Mode preserves the longer account session but removes privileged authority immediately.

## Rate limiting and privacy

Login uses both a normalized-email-plus-source bucket and a source-wide bucket. Registration uses a source-wide bucket. Parent unlock uses an authenticated session/user bucket. Implementations use rolling windows, return `429` with `Retry-After`, and provide the same response regardless of whether an email address exists. Deployment thresholds are configurable with conservative defaults and bounded values.

The source address is derived directly unless the request comes through the explicitly trusted private Caddy proxy. Arbitrary forwarded-address headers are not trusted. Authentication logs and audit metadata exclude passwords, PINs, cookies, CSRF tokens, bearer values, and password hashes.

## Household authorization

The family is the authorization boundary. Parent household reads and writes require an active, unrevoked account session in Parent Mode and an owner membership for that same family. Service queries receive the authorized family identifier from the server-side session principal; they never accept it from request JSON.

Registration creates the only MVP owner. Invitations, additional owners, account recovery, and multiple households per user remain outside the MVP. Household names are trimmed and validated at 1–80 characters. Timezones must resolve in the runtime IANA database. Week start accepts only Sunday or Monday. Timezone changes never rewrite stored occurrence local dates.

## Required security tests

- Transaction rollback leaves no user, family, membership, or session after any registration failure.
- Email normalization, duplicate and concurrent registration, generic login failure, and rate-limit behavior are covered.
- Session rotation, cookie attributes, logout revocation/clearing, absolute expiry, and each Parent Mode idle boundary are covered.
- Missing and invalid CSRF credentials fail without performing the mutation.
- Unauthenticated, shared, child, locked-parent, inactive-user, revoked, expired, wrong-household, and authorized-owner cases are covered.
- Unknown or cross-household identifiers use safe not-found behavior where disclosure would permit enumeration.
- Passwords, session credentials, CSRF values, and hashes never appear in logs or audit metadata.
