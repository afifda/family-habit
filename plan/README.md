# Family Habit Tracker Planning Index

These documents are the source of truth for the first production release.

- [Product requirements](./product-requirements.md): users, behavior, scope, and acceptance criteria.
- [Technical requirements](./technical-requirements.md): architecture, data, security, testing, and operations.
- [Roadmap](./roadmap.md): ordered development checklist and release gates.
- [Phase 0 decisions](./phase-0-decisions.md): proposed product, household, security, and points defaults awaiting owner approval.
- [Information architecture](./information-architecture.md): sitemap and primary journeys.
- [Wireframes](./wireframes.md): low-fidelity responsive screen specifications.
- [State machine](./state-machine.md): occurrence, completion, approval, and ledger transition contract.
- [Prototype specification](./prototype-specification.md): golden path, alternate paths, and review criteria.
- [Clickable prototype](./prototype/index.html): dependency-free Phase 0 interaction prototype; Parent Mode PIN `123456`.
- [Phase 0 review](./phase-0-review.md): PR-01–PR-10 prototype review and implementation caveats.

## Working agreement

Development follows the roadmap from top to bottom. A checklist item is complete only when its acceptance criteria, tests, and documentation are complete. Scope changes must first be reflected in the requirements and then added to the roadmap.

## Agreed MVP defaults

- A child submits work; a parent approves it before points are awarded.
- Child PINs are optional; Parent Mode is always protected.
- Supported schedules are daily, selected weekdays, and one-off tasks.
- Overdue one-off tasks remain visible until completed or cancelled.
- Points are informational in MVP; rewards and redemption are deferred.
- No negative points; corrections reverse or adjust ledger entries with an audit trail.
- Individual child progress is private by default.
- The application is online-only and hosted on one VPS.
- PostgreSQL is the system of record.
