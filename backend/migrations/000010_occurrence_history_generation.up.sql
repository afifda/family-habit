ALTER TABLE occurrences
  ADD COLUMN description_snapshot text NOT NULL DEFAULT '',
  ADD COLUMN icon_snapshot text NOT NULL DEFAULT '',
  ADD COLUMN color_snapshot text NOT NULL DEFAULT '';

ALTER TABLE habit_assignments
  DROP CONSTRAINT habit_assignments_supersedes_assignment_id_fkey,
  ADD CONSTRAINT assignments_supersedes_family_fk
    FOREIGN KEY (supersedes_assignment_id, family_id)
    REFERENCES habit_assignments(id, family_id) ON DELETE RESTRICT;

CREATE OR REPLACE FUNCTION prevent_occurrence_snapshot_rewrite() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (OLD.title_snapshot IS DISTINCT FROM NEW.title_snapshot OR
     OLD.description_snapshot IS DISTINCT FROM NEW.description_snapshot OR
     OLD.icon_snapshot IS DISTINCT FROM NEW.icon_snapshot OR
     OLD.color_snapshot IS DISTINCT FROM NEW.color_snapshot OR
     OLD.points_snapshot IS DISTINCT FROM NEW.points_snapshot OR
     OLD.item_type_snapshot IS DISTINCT FROM NEW.item_type_snapshot OR
     OLD.assignment_id IS DISTINCT FROM NEW.assignment_id OR
     OLD.task_id IS DISTINCT FROM NEW.task_id OR
     OLD.source_type IS DISTINCT FROM NEW.source_type OR
     OLD.child_id IS DISTINCT FROM NEW.child_id OR
     OLD.family_id IS DISTINCT FROM NEW.family_id OR
     OLD.local_date IS DISTINCT FROM NEW.local_date OR
     OLD.due_date IS DISTINCT FROM NEW.due_date) AND NOT (
       OLD.source_type='task' AND OLD.state='not_started' AND
       NOT EXISTS (SELECT 1 FROM completion_attempts WHERE occurrence_id=OLD.id)
     ) THEN
    RAISE EXCEPTION 'occurrence identity and snapshots are immutable';
  END IF;
  RETURN NEW;
END $$;
