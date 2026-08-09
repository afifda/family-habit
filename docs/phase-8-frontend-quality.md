# Phase 8 Frontend Quality Evidence

Date: 2026-08-09  
Scope: product integration, parent overview/reports/settings, responsive layout, and accessibility automation

## Implemented integration

- Parent Overview renders every active child with household-local daily completed/total progress, pending review count, balance, and direct Review/Reports navigation.
- Household Settings is a working parent-only route for household name, any backend-validated IANA timezone, Sunday/Monday week start, 5/15/30-minute Parent Mode timeout, and setting/removing a six-digit Parent Mode PIN.
- Reports provides daily/weekly/monthly navigation plus explicit balance, report, history, and point-activity loading/error/empty/retry states.
- Shared card, notice, dialog, focus, screen-reader-only, target, forced-colors, narrow reflow, and reduced-motion styles apply across parent and child routes.
- New Phase 8 overview/settings language is centralized in `src/content/messages.ts`. Pre-existing route literals remain localization-preparation debt and do not change core-flow meaning.

## Automated evidence

| ID | Evidence | Result |
|---|---|---|
| FE-QA-01 | `npm run format:check` | Pass |
| FE-QA-02 | `npm run typecheck` | Pass |
| FE-QA-03 | `npm run lint` | Pass, zero warnings |
| FE-QA-04 | `npm test -- --run` | Pass, 53/53 tests |
| FE-QA-05 | `npm run build` | Pass, production bundle without source maps |
| FE-OV-01 | Phase 8 component test: household-local completed/total and pending counts | Pass |
| FE-STATE-01 | Phase 8 component test: settings failure preserves input and retries | Pass |
| FE-RESP-01 | Phase 8 component test: primary Overview content at 320×568, 768×1024, and 1440×900 | Pass |
| A11Y-CONFIG-01 | `npm run test:e2e` | Pass, 24/24 across Chromium/Firefox/WebKit; 0 unexpected/flaky/skipped |

Playwright configuration writes its machine-readable result, HTML report, traces, screenshots, and videos below `artifacts/phase-8/`. The public-route browser suite includes axe serious/critical scans, logical visible keyboard focus, 320 CSS-pixel reflow, practical 48-pixel primary targets, and reduced-motion computation.

Browser binaries and system dependencies were installed and the complete public-route suite passed. Reproduce with:

```sh
cd frontend
npx playwright install --with-deps chromium firefox webkit
npm run build
npm run test:e2e
```

## State inventory

| Route family | Loading | Empty | Error/retry | Validation | Success/busy |
|---|---:|---:|---:|---:|---:|
| Parent Overview | yes | yes | global and per-child | n/a | n/a |
| Children | yes | yes | yes | yes | yes |
| Habits & Tasks | yes | yes | yes | yes | yes |
| Review | yes | yes | yes | decision guidance | announced/disabled |
| Reports | per resource | yes | per resource | correction/reversal | announced/aria-busy |
| Household Settings | yes | n/a | preserves input/retry | native and API | announced/aria-busy |
| Child Today/detail | skeleton | yes | permission/offline/retry | action state | live region/aria-busy |
| Child Points | yes | yes | yes | n/a | n/a |

## Required manual release matrix

These checks require real browser/device or assistive-technology observation and must not be inferred from component tests:

| ID | Configuration | Required result |
|---|---|---|
| A11Y-AT-01 | NVDA + current Firefox, child submission and parent approval | identity, groups, due state, points, statuses, dialogs announced accurately |
| A11Y-AT-02 | VoiceOver + current Safari/iOS, child submission and parent approval | same as above; touch exploration and software keyboard usable |
| A11Y-ZOOM-01 | 200% text and browser zoom on all core routes | no clipping, lost actions, or two-dimensional page scrolling |
| A11Y-REFLOW-02 | 400% zoom / 320 CSS-pixel equivalent | child Today and representative parent report remain coherent |
| A11Y-KB-02 | keyboard-only J-01, J-02, J-04, J-09 | logical order, visible focus, dialogs contained/restored, skip links work |
| RESP-PHONE-01 | 390×844 iOS Safari portrait/landscape | J-01, J-02, J-04 pass |
| RESP-PHONE-02 | 360×800 Android Chrome | J-02, J-06, offline/retry pass |
| RESP-TABLET-01 | 768×1024 and 1024×768 Safari/Chromium | J-01, J-04, J-07, J-09 pass |
| RESP-DESKTOP-01 | 1280×720 and 1440×900 Chromium/Firefox/WebKit | required desktop journeys pass |

Record tester, UTC timestamp, exact browser/OS/device versions, release commit or image digest, outcome, and artifact link for each row in the final Phase 8 validation record. No manual row is marked complete in this document.
