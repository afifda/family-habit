# Phase 8 Product Integration and Quality Contract

Status: **proposed release contract**  
Scope: PR-01 through PR-10, all non-functional requirements, and the Phase 8 roadmap gate  
Authority: product requirements, technical requirements, state machine, Phase 3–7 contracts, and Phase 1–7 validation records

## Purpose

Phase 8 proves that Habit Home works as one coherent household product. Earlier phase validation remains required, but isolated API, service, and component tests are not sufficient for this gate. The release candidate must demonstrate complete cross-role browser journeys, application-wide accessibility and responsive behavior, hardened browser/API delivery, safe operational output, and a traceable result for every MVP acceptance criterion.

The Phase 8 gate may be marked complete only from evidence produced against the same release candidate. A test name, command, scan report, screenshot/manual record, and any accepted exception must identify the tested commit or image digest and date.

## Current baseline and known gaps

The Phase 3–7 records establish strong authentication, tenant isolation, effective scheduling, completion concurrency, ledger integrity, reporting boundaries, and component-level behavior. The current frontend also contains Parent Overview and Reports routes, accessible dialog mechanics, visible focus styles, reduced-motion CSS, and responsive breakpoints.

The following are known Phase 8 blockers at contract creation time:

1. There is no Playwright configuration, browser package, or cross-role end-to-end suite.
2. There is no automated axe or equivalent accessibility scan in the frontend test toolchain.
3. Parent Overview does not show each child's household-local completed/total daily progress required by PR-09; it currently shows balance and a pending summary.
4. The Household Settings route is a placeholder, so the integrated household configuration journey is incomplete.
5. API responses have defensive headers, but Caddy/nginx do not yet establish the required headers for the HTML, JavaScript, CSS, and other SPA responses. HSTS is not yet present.
6. No Phase 8 dependency scan, container image scan, real-browser/device matrix, 200% text/zoom review, or screen-reader record exists.
7. User-facing strings are only partially centralized. This is not by itself a release blocker, but newly changed Phase 8 copy must use the shared content layer and the remaining exception must be recorded for the localization-preparation requirement.

These gaps are observations, not completion evidence. They remain open until the corresponding requirements below pass.

## Release evidence package

The release review must produce or link all of the following:

- a machine-readable E2E result and Playwright HTML report for Chromium, Firefox, and WebKit;
- automated accessibility results for every core route plus a manual accessibility record;
- a responsive/device and browser matrix with tester, date, viewport/device, and result;
- backend/frontend quality-command output, OpenAPI validation, migration-from-empty results, and live health checks;
- dependency and container scan reports with package/image versions and severity dispositions;
- an HTTP header and request-limit probe report covering both successful and error responses;
- a safe-log review containing synthetic-secret canaries and sanitized excerpts, never real credentials;
- a PR-01–PR-10 traceability report referencing exact automated tests or signed manual cases;
- a release-blocker register showing zero unresolved blockers.

Generated reports should live under a gitignored `artifacts/phase-8/` directory or an equivalent immutable release-artifact store. The durable summary and commands belong in `docs/phase-8-validation.md`.

## PR-01 through PR-10 traceability

Every row requires the prior-phase evidence and the Phase 8 integration evidence. “Prior” does not waive the integrated check.

| Requirement | Required Phase 8 proof | Primary evidence |
|---|---|---|
| PR-01 Parent authentication and household setup | Register atomically with Jakarta/Sunday defaults visible and editable; logout/login; read and update name, timezone, week start, and 5/15/30-minute idle setting; switching profile locks Parent Mode; idle expiry locks it; a child receives a server denial on every parent route. Confirm one-owner/no-invitation scope. | E2E J-01, J-05, and authorization matrix; Phase 3 integration tests; settings route tests |
| PR-02 Child management | Create and edit nickname/avatar/color/PIN; reject duplicate active nickname; archive without deleting history; archived child disappears from picker and remains in parent history. | E2E J-01 and J-06; Phase 4 database/HTTP tests |
| PR-03 Profile switching | Labelled large profile cards; PIN-free entry in at most two interactions; PIN flow; active identity visible and announced; switching clears parent privilege and browser Back cannot restore privileged data; Parent Mode requires password/PIN. | E2E J-02 and J-05; keyboard/screen-reader review; Phase 4 authorization tests |
| PR-04 Recurring habits | Create daily and selected-weekday habits for one/multiple children; points 1–10,000 validation; edit “this and future” with effective date; deactivate without rewriting progressed history. | E2E J-06/J-07; Phase 5 scheduling, DST, snapshot, and concurrency tests |
| PR-05 One-off tasks | Create a dated task; show on due date; keep overdue work neutral and actionable; cancel with reason; prove no task awards more than once. | E2E J-07/J-02/J-04; Phase 5 and Phase 7 integration/concurrency tests |
| PR-06 Child Today | Household-local To do/Waiting/Done groups; title, points, type, and due label; “I did it”; immediate accessible feedback and duplicate-tap protection; loading, empty, permission, offline/retry, and neutral-overdue states. | E2E J-02/J-03/J-10; automated axe; Child Today component tests; manual AT review |
| PR-07 Completion and approval | Submit without points; withdraw while pending; queue contains child/item/time/points; approve once; reject to not-started; repeat/concurrent requests remain idempotent; decisions are auditable. | E2E J-02/J-03/J-04/J-08; Phase 6–7 transaction/concurrency tests; audit reconciliation |
| PR-08 Points and history | Child sees only own balance/activity; parent sees balances and 30+ days; sibling detail denied; positive audited correction; reversal creates an exact compensating record and terminal occurrence. | E2E J-04/J-08/J-09; Phase 7 privacy, ledger, history, and hostile-write tests |
| PR-09 Parent overview and reports | Overview shows every active child, today's completed/total in household timezone, and pending count linked to Review. Day/week/month views show assigned, submitted, approved, rejected, incomplete, and points; Sunday/Monday weeks and local calendar months reconcile. | E2E J-04/J-09; overview/report component tests; Phase 7 timezone/report integration tests |
| PR-10 Responsive and accessible | Core child and parent journeys pass phone/tablet/desktop matrices, WCAG 2.2 AA automated checks, keyboard/focus review, 200% text/zoom, reduced motion, non-color labels, and screen-reader checks. Practical primary targets are at least 48×48 CSS px. | Automated accessibility suite; manual accessibility/device matrix; computed target/contrast evidence |

Traceability is complete only when every individual checkbox in `plan/product-requirements.md` maps to a passing case. A row-level assertion alone is insufficient. The validation document must use stable test IDs (for example, `E2E-J02-01`, `A11Y-KB-03`, `SEC-HDR-02`) so evidence can be rerun.

## Required end-to-end journeys

Tests use a fresh, isolated PostgreSQL database, real API, real browser, and production-built frontend behind the release proxy. Network mocking is allowed only in the explicit failure-state cases. Tests must create unique household data and must not depend on execution order.

### E2E-J01 — New household to first habit

Register an owner, verify/change household timezone and week start, enter Parent Mode, add a child with avatar/color and optional PIN, create and assign a recurring habit, and arrive at Overview. Verify the child and today's total appear. Target: the journey is operable with keyboard alone and the post-account child/habit setup can be completed in under three minutes during a documented usability run.

### E2E-J02 — Child submits work

Exit Parent Mode, verify Back does not reveal privileged content, select the child, enter an optional PIN, open Today, inspect an item, activate **I did it** once and with rapid repeated input, observe the accessible success announcement, and verify the item moves to Waiting without changing points.

### E2E-J03 — Child withdraws and resubmits

Withdraw a pending claim, verify it returns to To do, resubmit it, and confirm the audit/attempt history preserves both attempts. Attempt withdrawal after a parent decision and verify the action is absent and the API rejects a forced request.

### E2E-J04 — Parent reviews and awards once

Unlock Parent Mode, follow the Overview pending link, approve the submitted item, and verify queue removal, Overview progress, child Done state, point balance, activity, parent history, and report totals. Replay and race the approval at the API integration layer and reconcile exactly one award.

### E2E-J05 — Reject, retry, approve

Submit another item, reject it with child-safe wording, verify no points and return to To do, resubmit, approve, and verify complete attempt/audit history and exactly one award.

### E2E-J06 — Child lifecycle and privacy

Create two children, exercise duplicate nickname and PIN validation, switch between profiles, and prove each child can read only their own Today, points, and activity. Archive one child; verify picker removal, active-session downgrade, parent historical visibility, and no sibling/cross-household disclosure.

### E2E-J07 — Scheduling and one-off work

Create a multi-child weekday habit and a one-off task. Verify independent occurrences, due-today and neutral overdue labels, effective-date edit behavior, cancellation, deactivation, and unchanged historical snapshots. DST/weekday edge cases remain database integration tests and must run in the release suite.

### E2E-J08 — Reversal and correction

Reverse an approval after a clear confirmation and required reason, then verify terminal state, original award plus exact compensating entry, balance/report reconciliation, and inability to resubmit. Add a positive manual correction with confirmation; verify its reason is parent-only and its report attribution follows local creation date.

### E2E-J09 — Overview and reports

Seed mixed states across local day, week, and month boundaries. Verify per-child completed/total and household pending count on Overview; direct queue navigation; child switching; previous/next day/week/month; configured week start; all required counts and points; empty child and empty period states; 30-day incremental history.

### E2E-J10 — Resilience and session boundaries

For each core read route, exercise loading, empty, server error, offline, and retry. For risky mutations, abort the response after commit and verify retry reuses the idempotency key and displays canonical state without duplicate effects. Verify expired/revoked sessions, stale versions, permission denial, 404, and unknown routes provide an actionable, non-leaking recovery path.

### E2E-J11 — Household-local boundary

Run at least Jakarta and Berlin households with browser timezone deliberately different from household timezone. Verify Today date, Overview totals, due/overdue language, and report anchors/boundaries use the household setting. Include Berlin spring-gap and autumn-fold integration fixtures.

## Frontend state completeness

The following route families must have explicit automated or manual evidence for loading, empty, validation, success, generic server error, permission/session expiry, and retry where the state applies:

- authentication, registration, parent unlock, and profile picker;
- Parent Overview, Children, Habits & Tasks, Review, Reports, and Household Settings;
- Child Today, item detail, Child Points/activity, and profile switching;
- dialogs for archive, cancellation, approval/rejection, reversal, and correction;
- global not-found and offline/network failure behavior.

An error must not erase recoverable form input. An ambiguous mutation failure must retain its idempotency key until canonical success or an intentional new submission. Busy actions must be disabled and expose their busy state; success/error feedback must be programmatically announced without unexpectedly moving focus.

## Accessibility quality matrix

Core routes are `/`, `/login`, `/register`, `/parent/unlock`, `/parent`, `/parent/review`, `/parent/children`, `/parent/habits`, `/parent/reports`, `/parent/settings`, `/child/today`, and `/child/points` plus every modal state reached from them.

| Area | Automated requirement | Manual requirement | Pass condition |
|---|---|---|---|
| Semantics and names | axe-core scan after content settles, including open dialogs and validation/error states | Inspect landmark/heading order, lists/tables, field instructions, button/link names, current navigation, active child, and status names | Zero serious/critical violations; no material moderate violation; names match visible purpose |
| Keyboard | Browser test traverses menus, forms, cards, tabs, pagination, and dialogs | Keyboard-only completion of J-01, J-02, J-04, and J-09 with Tab/Shift+Tab/Enter/Space/arrows/Escape | No trap except contained modal; logical order; all actions reachable; skip link works |
| Focus | Assertions for dialog initial focus, containment, Escape, restoration, route heading/main focus, and post-mutation destination | Observe focus after validation, retry, profile switch, auth lock, list deletion, and status change | Focus is visible, predictable, and never lost behind removed content |
| Screen reader | DOM/name/state assertions and live-region tests | NVDA + Firefox on Windows and VoiceOver + Safari on current macOS/iOS for J-02 and J-04; at least one desktop AT pass for J-01/J-09 | Mode/identity, grouping, counts, due state, points, busy/error/success, dialog, and selected period are announced accurately |
| Contrast/non-color | Automated contrast scan for stable states | Inspect text, controls, focus, errors, selected states, charts/summary states, forced-colors mode | WCAG AA: 4.5:1 normal text, 3:1 large text/UI/focus; color/icon never sole carrier |
| Text/zoom/reflow | Viewport assertions at 320 CSS px and browser zoom where supported | 200% text and 200% browser zoom on core journeys; long nickname/title/reason and translated-length stress strings | No lost content/action, two-dimensional page scroll, clipping, or overlap except essential data regions |
| Targets/pointer | Computed-size check for primary child and common actions | Measure exceptions and spacing at phone width | 48×48 CSS px where practical; no target below WCAG 2.2 24×24 minimum without compliant spacing/exception |
| Motion | `prefers-reduced-motion` browser project | Inspect loading, dialogs, focus/hover, status movement | Non-essential motion/animation is removed; no flashing or motion-only feedback |
| Responsive orientation | Visual snapshots supplement assertions but are not sole evidence | Portrait/landscape phone and tablet, desktop, 400% zoom/reflow sampling | Content order and function remain coherent; no hover-only operation |

Automated accessibility is a guardrail, not a substitute for the manual screen-reader, focus, reflow, and language review. Any exception must cite a WCAG success criterion, impact, workaround, owner, and deadline; core-journey AA failures are blockers.

## Representative browser and device matrix

Testing uses the latest stable release available at validation time and records exact versions.

| Class | Minimum configuration | Journeys |
|---|---|---|
| Small phone | 320×568 CSS px, portrait, Chromium emulation | J-02, J-03, J-05, J-10 |
| Modern phone | 390×844 CSS px, portrait and landscape, real iOS Safari or equivalent device service | J-01, J-02, J-04; VoiceOver subset |
| Android phone | 360×800 CSS px, real Chrome Android or equivalent device service | J-02, J-06, offline/retry |
| Tablet | 768×1024 CSS px portrait and 1024×768 landscape, Safari and/or Chromium | J-01, J-04, J-07, J-09 |
| Desktop compact | 1280×720, Chromium and Firefox | Full automated suite, keyboard-only paths |
| Desktop standard | 1440×900 or larger, Chromium, Firefox, and WebKit/Safari | J-01, J-04, J-08, J-09 |
| Reflow | 1280 CSS px viewport at 400% zoom, equivalent to 320 CSS px content | J-02 and representative parent administration/report route |

Playwright Chromium, Firefox, and WebKit are mandatory automation projects. At least one real or hosted mobile Safari and one real or hosted Android Chrome run is mandatory because desktop emulation does not validate mobile browser behavior. Touch, software-keyboard field visibility, orientation, safe-area behavior, and scroll restoration are recorded manually.

## Security and privacy verification

### Browser and API headers

Probe document, hashed static asset, API success, API validation error, authentication error, and not-found responses through the public Caddy origin. Redirect HTTP to HTTPS in production mode. Required policy:

- `Strict-Transport-Security: max-age=31536000; includeSubDomains` on HTTPS production responses only; `preload` requires an explicit domain-wide decision;
- `Content-Security-Policy` for the SPA using explicit same-origin directives (`default-src 'self'`, no plugins/objects, no framing, restricted base/form/connect/image/font/style/script sources) compatible with the built bundle and without `unsafe-eval`; API may retain `default-src 'none'; frame-ancestors 'none'`;
- `X-Content-Type-Options: nosniff`;
- `Referrer-Policy: no-referrer` or a documented equally strict policy;
- `frame-ancestors 'none'` and `X-Frame-Options: DENY` as legacy defense;
- an explicit `Permissions-Policy` disabling unused sensitive capabilities;
- cache policy: HTML/authenticated API responses not stored by shared caches, immutable hashed assets cached long-term, session/cookie responses `no-store`;
- cookies remain `HttpOnly`, `Secure` in production, `SameSite=Lax`, and appropriately scoped.

Tests must assert that headers also appear on proxy-generated error responses. Do not enable HSTS on plain local HTTP in a way that impairs development.

### Request limits and abuse controls

- Preserve the API JSON-body ceiling of 1 MiB and add a test for exactly-under, over-limit, malformed, trailing JSON, unknown fields, slow/incomplete body, and declared/chunked transfers. Oversize input must fail without a state change.
- Configure conservative header size/read-header/read/write/idle timeouts on the Go server and body/time limits at Caddy where appropriate. Record chosen values.
- Retain login, parent-unlock, and child-PIN limiting; verify source and identity/session buckets, generic credential errors, `429`, recovery after the window, bounded limiter memory, and audit behavior.
- Apply proxy-level coarse request protection suitable for the single-family VPS. Do not introduce a limit that breaks valid E2E retries or accessibility clients.
- Verify pagination maxima, input length/range constraints, idempotency-key length, stale-version handling, and concurrent duplicate mutation behavior.
- Re-run the complete unauthenticated/parent/assigned-child/wrong-child/archived-child/cross-household authorization matrix against every route and method in OpenAPI.

### Safe-log and data-minimization review

Inject unique synthetic canaries as password, parent PIN, child PIN, cookie, CSRF token, Authorization value, idempotency key, email, child nickname, cancellation/rejection/correction reason, malformed body, and query value. Capture API, Caddy, frontend nginx, database/migration, and browser-console output. Fail if logs contain passwords/PINs, session cookies/tokens, CSRF values, request bodies, child reasons/notes, or credential hashes.

Normal request logs may include generated request ID, HTTP method, normalized route/path without query, status, duration, and documented safe actor/household identifiers. Email, IP, child identifiers/nicknames, and idempotency keys require explicit necessity, minimization/hashing, and retention documentation. Stack traces and SQL/database errors must not reach clients. Browser production builds must not log session projections or personal data and must not ship unintended source maps.

Verify the product collects no birth date, photo, address, school detail, public profile, or unnecessary analytics. Confirm siblings cannot view detailed progress and rejection/correction reasons are projected only to intended actors.

### Dependency, secret, and container scanning

Run all scanners against the release lockfiles and built image digests:

- `govulncheck ./...` for reachable Go vulnerabilities;
- `npm audit --audit-level=high` plus a lockfile-aware OSV scan for frontend packages;
- Trivy or Grype filesystem scan of the repository and image scan of API, frontend, Caddy, PostgreSQL, and migration images;
- secret scan of tracked content and image layers with Gitleaks or equivalent;
- container configuration review for pinned bases/digests, non-root execution, read-only filesystem/capability reduction where compatible, private API/database networking, health checks, and absence of embedded secrets/build credentials;
- SBOM generation for application images and retention with release artifacts.

Critical or high findings are blockers unless the scanner marks them demonstrably unreachable/not present and the security review records package/CVE, evidence, compensating control, owner, and expiry. “No fix available” alone is not an acceptance. Medium findings affecting authentication, tenant isolation, code execution, request parsing, the public proxy, or database integrity are also blockers. Suppressions must be exact, time-bounded, and reviewed; scanner outages do not count as a pass.

## Backend, database, and API release checks

The release candidate must pass:

- `gofmt` cleanliness, `go vet ./...`, `go test -race ./...`, and reproducible builds of API and migration commands;
- migrations 1–latest from an empty PostgreSQL 16 database and upgrade from the prior Phase 7 schema, with sqlc generation consistency;
- all database, authorization, DST/timezone, transaction rollback, idempotency, hostile-write, and concurrency suites;
- OpenAPI 3.1 parsing, local-reference resolution, implementation-route inventory, and client/contract consistency;
- health/readiness behavior, graceful shutdown, database interruption/recovery, and restart without data loss;
- live public-origin journeys through Caddy, not direct handler-only checks;
- household-scale performance sampling demonstrating p95 under 500 ms for core reads and 750 ms for core mutations, with dataset and method recorded.

No benchmark claim may be made from a mocked repository or warm single request.

## Frontend release checks

The release candidate must pass Prettier check, ESLint with zero warnings, TypeScript project checking, all Vitest/component tests, automated axe checks, production build, and the Playwright matrix. The production bundle must be checked for source maps, dev-only code, console errors, unhandled promise rejections, hydration/runtime errors, broken deep-link refreshes, cache/update behavior, and missing asset/route 404s.

Overview and Reports must reconcile against direct API results rather than client-derived mutable totals. Dates must remain household-local when browser and server timezones differ. Long text, empty datasets, pagination, delayed responses, and rapid repeated actions must not corrupt layout or state.

## Product language and visual review

- Maintain the approved calm, encouraging tone and Habit Home identity across authentication, child, and parent areas.
- Use **I did it**, **Waiting for parent**, and neutral overdue/rejection language; never imply shame, punishment, ranking, or sibling comparison.
- Icons, color, position, and animation supplement visible text rather than replace it.
- Destructive and point-changing parent actions use clear confirmation; routine child submission does not.
- Shared spacing, typography, color, control, card, notice, dialog, and responsive-navigation patterns must be consistent across every route.
- New and materially changed copy must be centralized. Record remaining pre-existing literal strings as localization debt; inconsistent or misleading core-flow copy is a blocker.

## Release blockers

Phase 8 cannot close while any of the following is true:

- any PR acceptance checkbox lacks a passing, reproducible evidence reference;
- Parent Overview lacks correct household-local completed/total and pending data, or daily/weekly/monthly reports fail to reconcile;
- a core journey cannot be completed on any mandatory browser/device class;
- a serious/critical automated accessibility violation, WCAG 2.2 AA core-journey failure, keyboard trap, inaccessible status, lost focus, or content/action loss at required reflow remains;
- a child, wrong child, archived child, unauthenticated actor, or another household can access or mutate forbidden data;
- duplicate points, non-atomic decisions, rewritten history, idempotency failure, report boundary error, or migration failure is observed;
- any critical/high security or container/dependency vulnerability remains without an approved evidence-based exception, or a security-relevant medium finding remains unresolved;
- SPA/API security headers, production cookie properties, request limits/timeouts, CSRF, or rate-limit behavior fail their probes;
- logs, client errors, assets, or images expose a credential, session/CSRF secret, PIN/password/hash, private reason/note, or embedded production secret;
- E2E, accessibility, browser/device, scan, OpenAPI, backend, frontend, Docker, health, or release-artifact evidence is missing, stale, or produced from a different candidate;
- a placeholder, dead end, broken retry, or permission leak remains in a core MVP route.

Lower-severity cosmetic defects may be deferred only when they do not affect comprehension, accessibility, privacy, integrity, or completion of a core journey. Each deferral needs impact, owner, target phase/date, and product approval.

## Gate acceptance

The Phase 8 roadmap gate is accepted only when:

1. every PR-01–PR-10 acceptance criterion is mapped to passing automated or documented manual evidence;
2. E2E-J01 through E2E-J11 pass against the production-shaped release candidate;
3. the accessibility and browser/device matrices are complete with no core WCAG 2.2 AA blocker;
4. security headers, request controls, authorization, safe logs, data minimization, and dependency/container scans pass;
5. overview and report values reconcile with household-local API/database truth;
6. all backend, frontend, OpenAPI, migration, Docker, health, and performance checks pass; and
7. the release-blocker register contains zero unresolved items.

Only then may `docs/phase-8-validation.md` record Phase 8 as complete and the roadmap advance to VPS production launch. Production provisioning, DNS, HTTPS issuance for the real domain, monitoring, backups, and restore rehearsal remain Phase 9, but Phase 8 must prove that the application artifacts and proxy policy are ready for that work.
