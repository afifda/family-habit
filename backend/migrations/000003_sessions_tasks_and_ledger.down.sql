DROP TABLE IF EXISTS idempotency_records;
ALTER TABLE audit_events
  DROP CONSTRAINT IF EXISTS audit_exactly_one_actor,
  ADD CONSTRAINT audit_actor_present CHECK (actor_user_id IS NOT NULL OR actor_child_id IS NOT NULL),
  DROP COLUMN IF EXISTS idempotency_key,
  DROP COLUMN IF EXISTS after_status,
  DROP COLUMN IF EXISTS before_status,
  DROP COLUMN IF EXISTS session_id,
  ALTER COLUMN family_id DROP NOT NULL;
DROP TRIGGER IF EXISTS point_ledger_reversal_guard ON point_ledger;
DROP FUNCTION IF EXISTS validate_ledger_reversal();
DROP INDEX IF EXISTS point_ledger_one_reversal_source;
ALTER TABLE point_ledger
  DROP CONSTRAINT IF EXISTS ledger_attempt_scope_fk,
  DROP CONSTRAINT IF EXISTS ledger_kind_shape,
  DROP COLUMN IF EXISTS reverses_entry_id,
  ADD CONSTRAINT ledger_kind_shape CHECK (
    (kind = 'award' AND occurrence_id IS NOT NULL AND completion_attempt_id IS NOT NULL AND points > 0) OR
    (kind = 'approval_reversal' AND occurrence_id IS NOT NULL AND completion_attempt_id IS NULL AND points < 0) OR
    (kind = 'manual_correction' AND occurrence_id IS NULL AND completion_attempt_id IS NULL AND btrim(coalesce(reason, '')) <> '')
  );
ALTER TABLE completion_attempts DROP CONSTRAINT IF EXISTS attempts_id_scope_unique;
ALTER TABLE habit_schedules DROP CONSTRAINT IF EXISTS schedules_shape_valid;
ALTER TABLE habit_schedules ADD CONSTRAINT schedules_shape_valid CHECK (
  (kind = 'daily' AND cardinality(weekdays) = 0 AND due_date IS NULL) OR
  (kind = 'weekdays' AND cardinality(weekdays) BETWEEN 1 AND 7 AND due_date IS NULL) OR
  (kind = 'one_off' AND cardinality(weekdays) = 0 AND due_date IS NOT NULL)
);
DROP INDEX IF EXISTS occurrences_one_per_task;
ALTER TABLE occurrences
  DROP CONSTRAINT IF EXISTS occurrences_version_positive,
  DROP CONSTRAINT IF EXISTS occurrences_source_shape,
  DROP CONSTRAINT IF EXISTS occurrences_task_scope_fk,
  DROP COLUMN IF EXISTS version,
  DROP COLUMN IF EXISTS item_type_snapshot,
  DROP COLUMN IF EXISTS due_date,
  DROP COLUMN IF EXISTS source_type,
  DROP COLUMN IF EXISTS task_id,
  ALTER COLUMN assignment_id SET NOT NULL;
DROP TABLE IF EXISTS one_off_tasks;
DROP TABLE IF EXISTS sessions;
DROP TYPE IF EXISTS task_state;
DROP TYPE IF EXISTS session_mode;
ALTER TYPE ledger_kind RENAME VALUE 'approval_reversal' TO 'reversal';
ALTER TYPE ledger_kind RENAME VALUE 'manual_correction' TO 'correction';
ALTER TABLE families
  DROP CONSTRAINT IF EXISTS families_parent_idle_valid,
  DROP COLUMN IF EXISTS parent_idle_minutes,
  ALTER COLUMN week_starts_on SET DEFAULT 1,
  ALTER COLUMN timezone SET DEFAULT 'Europe/Berlin';

