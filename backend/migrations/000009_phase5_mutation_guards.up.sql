ALTER TABLE habits
  ADD COLUMN version bigint NOT NULL DEFAULT 1,
  ADD CONSTRAINT habits_version_positive CHECK (version > 0);

ALTER TABLE habit_assignments
  ADD COLUMN version bigint NOT NULL DEFAULT 1,
  ADD CONSTRAINT assignments_version_positive CHECK (version > 0);

ALTER TABLE one_off_tasks
  ADD COLUMN version bigint NOT NULL DEFAULT 1,
  ADD CONSTRAINT tasks_version_positive CHECK (version > 0);

ALTER TABLE audit_events
  ADD COLUMN reason text,
  ADD CONSTRAINT audit_reason_valid CHECK (reason IS NULL OR (reason=btrim(reason) AND char_length(reason) BETWEEN 1 AND 500));
