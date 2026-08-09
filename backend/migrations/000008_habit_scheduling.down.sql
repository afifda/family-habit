DROP TRIGGER IF EXISTS occurrences_snapshot_immutable ON occurrences;
DROP FUNCTION IF EXISTS prevent_occurrence_snapshot_rewrite();
DROP INDEX IF EXISTS one_off_tasks_child_due;
ALTER TABLE completion_attempts DROP CONSTRAINT IF EXISTS attempts_decision_shape;
ALTER TABLE completion_attempts ADD CONSTRAINT attempts_decision_shape CHECK (
  (decision = 'pending' AND decided_at IS NULL AND decided_by IS NULL) OR
  (decision = 'withdrawn' AND decided_at IS NOT NULL AND decided_by IS NULL) OR
  (decision IN ('approved','rejected') AND decided_at IS NOT NULL AND decided_by IS NOT NULL)
);
ALTER TABLE habit_assignments DROP CONSTRAINT IF EXISTS assignments_no_effective_overlap;
ALTER TABLE habit_assignments DROP COLUMN IF EXISTS supersedes_assignment_id;
ALTER TABLE habits DROP CONSTRAINT IF EXISTS habits_color_valid;
ALTER TABLE habits DROP CONSTRAINT IF EXISTS habits_icon_valid;
ALTER TABLE habits DROP CONSTRAINT IF EXISTS habits_description_valid;
ALTER TABLE habits DROP CONSTRAINT IF EXISTS habits_title_valid;
ALTER TABLE habits DROP COLUMN IF EXISTS inactive_from;
DROP TABLE IF EXISTS habit_versions;
