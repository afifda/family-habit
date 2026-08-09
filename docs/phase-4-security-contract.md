# Phase 4 Child Profiles and Secure Switching Contract

This document records the security and authorization decisions for PR-02 and PR-03. The OpenAPI document is the HTTP source of truth; implementations must preserve the boundaries below when the contract is updated.

## Identity and profile projections

The authenticated account session belongs to the household owner and may be in exactly one mode: shared profile picker, Parent Mode, or one active child. Child mode is a restricted view within that account session, not an independent account or proof of a child's real-world identity.

The shared profile picker may read active children in its own household. Its deliberately limited projection contains only the child ID, nickname, bundled avatar key, color, and whether a PIN challenge is required. It never exposes PIN hashes, archival metadata, activity history, timestamps, or parent-only settings. Parent child-management responses may additionally expose active or archived status, administrative timestamps, and `pinEnabled`, but never a PIN or verifier.

Archived children are excluded from the shared profile projection. Parents may explicitly include archived children in the management list so that retained history remains understandable.

## Child-session transitions

Entering child mode requires a valid, active account session and an active child belonging to that session's household. The server obtains the household from the authenticated session and never from request input. If the child has a PIN hash, the request must include a matching 4–6 digit PIN; if no PIN is configured, selecting the profile is sufficient and remains a trusted-device convenience rather than strong identity verification.

The transition to child mode atomically sets the one active child and clears Parent Mode state, including the parent unlock timestamp and idle authority. The session response reports `actor: child` and the exact `childId`. Leaving child mode atomically clears the active child and returns the session to the shared profile picker. All session-mode mutations require the session-bound CSRF credential.

Child authorization requires both child mode and an exact match between the session's active child and the resource child. Merely possessing the household account cookie or being in child mode is insufficient. Parent operations require current Parent Mode and the owner membership for the same household. Route-aware middleware is supported by a service or transactional authorization check at sensitive mutation time.

## PIN handling, rate limiting, and privacy

Child PINs contain 4–6 ASCII digits. They are optional and are stored only as slow Argon2id hashes. Create accepts an omitted PIN as disabled. Update uses three distinct states: omission preserves the existing verifier, `null` removes it, and a valid string replaces it. Raw PINs and hashes must never appear in API responses, application logs, request metadata, audit metadata, or client persistence.

Failed child-entry attempts are rate-limited by authenticated session and child, with a source-wide limit to prevent cycling through profiles. Limits use rolling windows, bounded configuration, and `Retry-After` on `429` responses. Unknown, archived, cross-household, and wrong-PIN child-entry attempts use the same generic response shape and message so that the endpoint does not become a profile or PIN oracle. Audit records contain only safe actor, household, child, session, source-hash, outcome, and timestamp fields.

## Child data and archival integrity

Nicknames are trimmed before validation and storage and contain 1–40 characters. Active nicknames are unique within a household after case folding and trimming. Avatar keys are restricted to the approved bundled set, and colors use the documented six-digit hexadecimal form. Database constraints preserve these rules even if a new code path bypasses HTTP validation.

Archival is a soft state transition. It hides the child from the normal picker but retains the child row and all related occurrences, attempts, ledger entries, and audit history. Archiving and the downgrade of every active session for that child occur in the same database transaction. A session whose active child has nevertheless become invalid or archived is downgraded to shared mode during authentication before it can receive child authority.

Archive requests are idempotent for an existing child in the authorized household: the first and repeated request return `204`. Unknown and cross-household identifiers return the same safe `404`. A nickname becomes available for a new active child after the previous child is archived, while the archived record keeps its original nickname and history.

## Status and error semantics

- Missing, invalid, expired, or revoked account authentication returns `401 unauthorized`.
- A valid session with a missing or invalid CSRF credential returns `403 csrf_invalid`.
- A valid actor without the required mode or role returns `403 forbidden` or the documented `parent_mode_required` specialization.
- Parent CRUD returns `404 not_found` for unknown and cross-household child identifiers.
- Child entry deliberately returns the same generic credential/profile failure for unknown, archived, cross-household, or incorrect-PIN input.
- An active nickname collision returns `409 nickname_conflict`; unrelated constraint failures are not mislabeled as nickname conflicts.
- Invalid nickname, avatar, color, or PIN input returns `422 validation_failed` with field-level issues.
- An exceeded PIN-attempt limit returns `429 rate_limited` and includes `Retry-After`.

Errors must not reveal whether another household contains an identifier or whether a child has a particular PIN. Responses and logs must not contain database constraint text, secrets, or credential hashes.

## Concurrency boundary

Authorization established at the start of an HTTP request is not sufficient for a privileged mutation that can race with a profile switch, idle lock, logout, or archive. Sensitive parent and child mutations re-check the persisted session mode, active child, revocation and expiry state, household scope, and relevant active-record state within the mutation transaction or an equivalent atomic conditional statement.

Child entry uses an atomic conditional transition so that it cannot select a cross-household or archived child between validation and update. Archival locks or conditionally updates the child and downgrades matching sessions in the same transaction. Concurrent switches have a single persisted winner; clients treat the returned session as authoritative and refresh when another tab changes the mode. Switching the UI before the server accepts the transition is not permitted.

## Authorization and release gate matrix

The Phase 4 gate requires integration coverage for each meaningful operation against unauthenticated, shared-profile, same-child, wrong-child, archived-child, locked-parent, active owner-parent, and cross-household-parent cases.

| Operation | Shared profile | Active child | Active owner parent | Required negative coverage |
| --- | --- | --- | --- | --- |
| Read profile projection | Own household active children | Own household active children | Own household active children | Unauthenticated; archived omission; cross-household data absence |
| List/manage children | Denied | Denied | Allowed for own household | Locked parent; unknown ID; cross-household ID; CSRF on mutations |
| Enter child without configured PIN | Allowed for own active child | Allowed switch to own active household child | Allowed and immediately locks Parent Mode | Archived, unknown, and cross-household child |
| Enter child with configured PIN | Allowed after correct PIN | Allowed after correct PIN | Allowed after correct PIN and immediately locks Parent Mode | Missing/wrong PIN; rate limit; generic failure privacy |
| Leave child mode | Idempotently shared | Allowed, resulting in shared mode | Parent lock uses its dedicated transition | Missing/invalid CSRF and revoked session |
| Read or mutate child-scoped resources | Denied unless explicitly shared | Exact active child only | According to the endpoint's parent policy | Wrong child, archived active child, and cross-household resource |
| Read or mutate parent resources | Denied | Denied | Allowed while Parent Mode remains active | Idle lock, concurrent switch, logout, and cross-household resource |

The release gate also requires:

- Unit tests for validation, PIN update tri-state behavior, hashing and verification, limiter boundaries, and session serialization.
- PostgreSQL integration tests for trimmed case-insensitive uniqueness, reuse after archive, retained history, session-mode constraints, archival downgrade, and concurrent archive/switch behavior.
- HTTP tests for authentication, CSRF, exact-child and household scope, generic PIN failures, rate-limit headers, accurate session actor output, and secret-free responses and logs.
- Frontend component and accessibility tests for picker loading, empty, error, retry and PIN states; keyboard and touch operation; active-profile identification; child editing; and archive confirmation.
- An end-to-end shared-device flow that creates a child, enters both PIN-free and PIN-protected profiles, proves parent APIs are denied in child mode, returns to the picker, unlocks Parent Mode, edits and archives the child, and confirms archived history remains parent-visible while the profile disappears from the picker.

Phase 4 is complete only when fresh migrations, backend and frontend quality checks, the authorization matrix, and the live shared-device flow all pass.
