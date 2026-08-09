CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE habit_versions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL,
  habit_id uuid NOT NULL,
  title text NOT NULL,
  description text,
  icon text,
  color text,
  effective_from date NOT NULL,
  effective_until date,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT habit_versions_habit_scope_fk FOREIGN KEY (habit_id, family_id)
    REFERENCES habits(id, family_id) ON DELETE RESTRICT,
  CONSTRAINT habit_versions_title_valid CHECK (title=btrim(title) AND char_length(title) BETWEEN 1 AND 120),
  CONSTRAINT habit_versions_description_valid CHECK (description IS NULL OR char_length(description) <= 500),
  CONSTRAINT habit_versions_icon_valid CHECK (icon IS NULL OR char_length(icon) <= 40),
  CONSTRAINT habit_versions_color_valid CHECK (color IS NULL OR color ~ '^#[0-9A-Fa-f]{6}$'),
  CONSTRAINT habit_versions_dates_valid CHECK (effective_until IS NULL OR effective_until >= effective_from),
  EXCLUDE USING gist (
    habit_id WITH =,
    daterange(effective_from, coalesce(effective_until + 1, 'infinity'::date), '[)') WITH &&
  )
);
CREATE INDEX habit_versions_effective ON habit_versions(habit_id,effective_from DESC);

INSERT INTO habit_versions(family_id,habit_id,title,description,icon,color,effective_from)
SELECT family_id,id,title,description,icon,color,'0001-01-01'::date FROM habits;

ALTER TABLE habits
  ADD COLUMN inactive_from date,
  ADD CONSTRAINT habits_title_valid CHECK (title=btrim(title) AND char_length(title) BETWEEN 1 AND 120),
  ADD CONSTRAINT habits_description_valid CHECK (description IS NULL OR char_length(description) <= 500),
  ADD CONSTRAINT habits_icon_valid CHECK (icon IS NULL OR char_length(icon) <= 40),
  ADD CONSTRAINT habits_color_valid CHECK (color IS NULL OR color ~ '^#[0-9A-Fa-f]{6}$');

ALTER TABLE habit_assignments
  ADD COLUMN supersedes_assignment_id uuid REFERENCES habit_assignments(id) ON DELETE RESTRICT,
  ADD CONSTRAINT assignments_no_effective_overlap EXCLUDE USING gist (
    habit_id WITH =,
    child_id WITH =,
    daterange(effective_from, coalesce(effective_until + 1, 'infinity'::date), '[)') WITH &&
  );

CREATE INDEX one_off_tasks_child_due ON one_off_tasks(family_id,child_id,due_date)
  WHERE state='active';

ALTER TABLE completion_attempts DROP CONSTRAINT attempts_decision_shape;
ALTER TABLE completion_attempts ADD CONSTRAINT attempts_decision_shape CHECK (
  (decision = 'pending' AND decided_at IS NULL AND decided_by IS NULL) OR
  (decision IN ('withdrawn','cancelled') AND decided_at IS NOT NULL AND decided_by IS NULL) OR
  (decision IN ('approved','rejected') AND decided_at IS NOT NULL AND decided_by IS NOT NULL)
);

CREATE OR REPLACE FUNCTION prevent_occurrence_snapshot_rewrite() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (OLD.title_snapshot IS DISTINCT FROM NEW.title_snapshot OR
     OLD.points_snapshot IS DISTINCT FROM NEW.points_snapshot OR
     OLD.item_type_snapshot IS DISTINCT FROM NEW.item_type_snapshot OR
     OLD.assignment_id IS DISTINCT FROM NEW.assignment_id OR
     OLD.task_id IS DISTINCT FROM NEW.task_id OR
     OLD.source_type IS DISTINCT FROM NEW.source_type OR
     OLD.child_id IS DISTINCT FROM NEW.child_id OR
     OLD.family_id IS DISTINCT FROM NEW.family_id) AND NOT (
       OLD.source_type='task' AND OLD.state='not_started' AND
       NOT EXISTS (SELECT 1 FROM completion_attempts WHERE occurrence_id=OLD.id)
     ) THEN
    RAISE EXCEPTION 'occurrence identity and snapshots are immutable';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER occurrences_snapshot_immutable
  BEFORE UPDATE ON occurrences FOR EACH ROW EXECUTE FUNCTION prevent_occurrence_snapshot_rewrite();
