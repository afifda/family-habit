# Contributing

## Workflow

1. Read the product and technical requirements in `plan/`.
2. Work on the next unchecked roadmap item without expanding its scope silently.
3. Add or update tests with every behavioral change.
4. Run `make check` before requesting review.
5. Update documentation and roadmap evidence when the work genuinely satisfies it.

Use small focused changes. Never commit `.env` files, credentials, child data, database dumps, generated build output, or dependency directories.

## Code expectations

- Go domain rules belong outside HTTP handlers.
- Frontend controls must be keyboard accessible and must not communicate status by color alone.
- Every household-owned backend query must eventually be scoped by household ID.
- Point-changing operations require transaction and idempotency tests when implemented.
- User-facing dates use the household timezone; stored timestamps use UTC.

