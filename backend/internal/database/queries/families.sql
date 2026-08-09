-- name: GetFamily :one
SELECT id, name, timezone, week_starts_on, created_at, updated_at
FROM families WHERE id = $1;

-- name: CreateFamily :one
INSERT INTO families (name, timezone, week_starts_on)
VALUES ($1, $2, $3)
RETURNING id, name, timezone, week_starts_on, created_at, updated_at;

-- name: ListActiveChildren :many
SELECT id, family_id, nickname, avatar, color, pin_hash, archived_at, created_at, updated_at
FROM children WHERE family_id = $1 AND archived_at IS NULL ORDER BY nickname;

-- name: ChildPointBalance :one
SELECT coalesce(sum(points), 0)::bigint AS balance
FROM point_ledger WHERE family_id = $1 AND child_id = $2;
