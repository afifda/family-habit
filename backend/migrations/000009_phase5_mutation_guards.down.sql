ALTER TABLE audit_events DROP CONSTRAINT audit_reason_valid, DROP COLUMN reason;
ALTER TABLE one_off_tasks DROP CONSTRAINT tasks_version_positive, DROP COLUMN version;
ALTER TABLE habit_assignments DROP CONSTRAINT assignments_version_positive, DROP COLUMN version;
ALTER TABLE habits DROP CONSTRAINT habits_version_positive, DROP COLUMN version;
