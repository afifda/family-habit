# Habit Home frontend

React and TypeScript client for Habit Home. It includes the complete parent and
child MVP routes, server-state configuration, runtime environment validation,
responsive design tokens, component tests, and cross-browser accessibility tests.

## Local development

1. Install Node.js 22 or newer.
2. Copy `.env.example` to `.env.local` if the default `/api/v1` path is unsuitable.
3. Run `npm install`.
4. Run `npm run dev` and open `http://localhost:5173`.

Vite proxies `/api` to `http://localhost:8080` in development. Browser-visible
environment variables are not secrets.

## Quality commands

- `npm run format:check`
- `npm run lint`
- `npm run typecheck`
- `npm test`
- `npm run build`
- `npx playwright install --with-deps chromium firefox webkit` (first browser run)
- `npm run test:e2e`

Playwright runs Chromium, Firefox, and WebKit and stores reports and failure
artifacts under `../artifacts/phase-8`. Build the frontend before running the
browser suite; set `PLAYWRIGHT_BASE_URL` to exercise an already running
production-shaped public origin.

Feature-specific components should live in `src/features`; shared UI belongs in
`src/components`; API access belongs in `src/api`. Centralize user-facing copy in
`src/content` so later localization does not require searching component markup.
