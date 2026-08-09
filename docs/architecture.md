# Architecture

Habit Home begins as a modular monolith: one React application, one Go API, and one PostgreSQL database behind Caddy. This keeps deployment and transactions simple while preserving domain boundaries inside the API.

Backend packages are organized around `auth`, `family`, `children`, `habits`, `completions`, and `points`. HTTP transport and health/configuration infrastructure remain separate from domain logic. Phase 2 will add PostgreSQL migrations, `pgx`, typed queries, OpenAPI, and database-backed readiness.

The frontend uses route-level parent and child shells, TanStack Query for server state, and shared accessible visual tokens. Browser route guards will improve usability, but backend authorization will remain authoritative.

Compose exposes only Caddy publicly in the production-shaped path. PostgreSQL, the API, and the frontend remain on a private network. The API port is bound to loopback for local development diagnostics.

