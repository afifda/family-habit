# Phase 8 Validation Record

Date: 2026-08-09  
Status: **in progress — release gate remains blocked**

This record separates completed automated evidence from the manual and production-shaped checks that cannot honestly be inferred from unit or emulated-browser tests.

## Completed evidence

| Area | Result |
|---|---|
| Backend | Go tests, vet, API build, and migration build pass in the pinned Go container |
| Frontend | format, ESLint, TypeScript, 53/53 Vitest tests, and production build pass |
| Browser/a11y baseline | 24/24 Playwright checks pass in Chromium, Firefox, and WebKit with zero unexpected, flaky, or skipped tests |
| Database journey | real PostgreSQL HTTP journey passes: register → child → recurring assignment → Today → submit → parent approval → points/report |
| Runtime | PostgreSQL, migrations, API, frontend, and Caddy start from Compose; API and frontend health checks pass |
| Security headers | CSP, frame denial, no-sniff, referrer, permissions, COOP/CORP, HSTS policy, HTML/API no-store, and immutable hashed assets configured |
| Request/log safety | 1 MiB body cap, malformed/unknown/trailing JSON tests, bounded limiter, sanitized request IDs, and query-free structured request logging |
| Dependencies | npm audit reports 0 vulnerabilities; Go dependencies upgraded to patched pgx/crypto/text releases |
| Filesystem | Trivy vulnerability/secret/misconfiguration scan reports zero high/critical findings |
| Images | Trivy reports zero high/critical findings for API, frontend, custom Caddy, and custom PostgreSQL images |
| Supply chain | CycloneDX SBOMs generated for API and frontend images under `artifacts/phase-8/` |

## PR-01–PR-10 traceability

| Requirement | Automated/documented evidence | Remaining release evidence |
|---|---|---|
| PR-01 household/auth/settings | auth tests, real Settings route, PostgreSQL journey | authenticated browser journey and manual AT |
| PR-02 children | Phase 4 service/HTTP/component tests | real-device archive/editor observation |
| PR-03 profile switching | Phase 4 authorization and picker tests | keyboard/screen-reader shared-device journey |
| PR-04 habits | Phase 5 database/concurrency/component tests | authenticated browser journey |
| PR-05 tasks | Phase 5 edit/cancel/state tests | authenticated browser journey |
| PR-06 Today | Phase 6 service/concurrency/component tests | authenticated axe and real-device child flow |
| PR-07 approval | Phase 7 atomicity/idempotency tests and PostgreSQL journey | authenticated browser focus/AT flow |
| PR-08 points/reports | Phase 7 reconciliation tests and report UI tests | authenticated browser pagination/correction flow |
| PR-09 overview | authoritative aggregate API and Phase 8 component tests | production-shaped browser verification |
| PR-10 accessibility/responsive | public-route axe, keyboard, 320px, target, reduced-motion across three engines | mandatory manual AT, zoom, orientation, and real-device matrix |

## Open release blockers

- Implement and pass authenticated browser journeys E2E-J01–J11 through Caddy against the real API/database.
- Run axe on authenticated parent/child routes, dialogs, and representative error states; review material moderate findings.
- Complete and record NVDA/Firefox and VoiceOver/iOS checks plus Android Chrome, keyboard-only core journeys, 200% text/zoom, 400% reflow, orientation, and software-keyboard checks.
- Validate production-domain HTTPS redirect and HSTS after Phase 9 DNS/TLS configuration.
- Retain candidate-bound scan logs/digests and add the remaining OSV/Gitleaks evidence required by the strict quality contract.
- Record public-origin performance p95 sampling and a complete OpenAPI authorization inventory.

The Phase 8 roadmap gate intentionally remains unchecked until this blocker list is empty.
