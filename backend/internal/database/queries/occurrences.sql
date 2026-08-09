-- name: ListChildOccurrencesForDate :many
SELECT id, family_id, assignment_id, child_id, local_date, title_snapshot,
       points_snapshot, state, created_at, updated_at
FROM occurrences
WHERE family_id = $1 AND child_id = $2 AND local_date = $3
ORDER BY created_at, id;

-- name: CreateOccurrence :one
INSERT INTO occurrences
  (family_id, assignment_id, child_id, local_date, title_snapshot, points_snapshot)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (assignment_id, child_id, local_date) DO UPDATE
  SET title_snapshot = occurrences.title_snapshot
RETURNING id, family_id, assignment_id, child_id, local_date, title_snapshot,
          points_snapshot, state, created_at, updated_at;
