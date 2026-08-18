# Habit Home

Habit Home is a private, child-friendly family habit tracker. Parents manage children, recurring habits, one-off tasks, approvals, reports, and points; children use a focused shared-device experience to see and submit today's work.

Current status: Phase 8 automated integration and quality work is substantially complete, with its manual release evidence still open. Phase 9 dynamic routines and optional rewards is approved for immediate implementation ahead of VPS launch. See the [roadmap](plan/roadmap.md) and [feature plan](plan/routine-groups-and-rewards.md).

## Stack

- Go 1.26 modular API
- React 19, TypeScript, and Vite
- PostgreSQL 16
- Docker Compose and Caddy

```text
Browser → Caddy → React static site
                └→ Go API → PostgreSQL
```

## Repository

- `backend/`: Go API, internal modules, tests, migrations, and OpenAPI workspace
- `frontend/`: React application and component tests
- `deploy/`: local/production-shaped Compose and Caddy configuration
- `docs/`: architecture and contribution guidance
- `plan/`: approved requirements, decisions, prototype, and roadmap

## Prerequisites

- Go 1.26+
- Node.js 22+ and npm 9+
- Docker Engine with Docker Compose v2
- GNU Make

## First run

```bash
cp deploy/.env.example deploy/.env
cd frontend && npm ci && cd ..
make up
```

Open `http://localhost`. The API health endpoints are available through `http://localhost/health/live` and `http://localhost/health/ready`.

For faster frontend work, keep PostgreSQL and the API in Compose, stop the `frontend` and `caddy` services if needed, and run `make frontend-dev`; Vite listens on `http://localhost:5173` and proxies `/api` to port 8080.

## Commands

| Command | Purpose |
|---|---|
| `make setup` | Install locked frontend dependencies and create local deploy environment |
| `make fmt` | Format Go and frontend files |
| `make check` | Run formatting, linting, type checks, tests, builds, and Compose validation |
| `make frontend-dev` | Start Vite locally |
| `make api-dev` | Start the API locally; its database URL must resolve from the host |
| `make up` | Build and start the full Compose stack |
| `make down` | Stop the stack without deleting data |
| `make logs` | Follow service logs |

## Configuration

| Variable | Service | Development value | Notes |
|---|---|---|---|
| `APP_ENV` | API | `development` | Runtime environment |
| `HTTP_ADDR` | API | `:8080` | API listen address |
| `DATABASE_URL` | API | Compose-generated | PostgreSQL URL; production must use separate secrets and TLS settings |
| `READINESS_TIMEOUT` | API | `2s` | Database readiness timeout |
| `SHUTDOWN_TIMEOUT` | API | `10s` | Graceful shutdown deadline |
| `POSTGRES_DB` | Compose | `family_habit` | Local database name |
| `POSTGRES_USER` | Compose | `family_habit` | Local database user |
| `POSTGRES_PASSWORD` | Compose | local placeholder | Never reuse in production |
| `PUBLIC_ORIGIN` | Caddy | `http://localhost` | Site address; use the HTTPS domain in production |
| `VITE_API_BASE_URL` | Browser | `/api/v1` | Public browser configuration, never a secret |

Copy example environment files rather than committing real values. Anything prefixed with `VITE_` is visible to browsers.

## Quality and planning

CI uses the same format, lint, test, type-check, build, dependency-audit, and Compose-validation commands documented here. Read [CONTRIBUTING.md](docs/CONTRIBUTING.md) before changing code and update the [roadmap](plan/roadmap.md) only when its evidence is complete.

This application contains children's private family data. Do not commit credentials, database exports, production logs, or personal data.

## Troubleshooting

- Port already used: stop the process using ports 80, 443, 5173, or 8080, or adjust the local configuration.
- API not ready: run `make logs` and wait for PostgreSQL's health check.
- Clean development data: `docker compose --env-file deploy/.env -f deploy/compose.yaml down --volumes` permanently deletes the local database and Caddy volumes. Never run it against production.
