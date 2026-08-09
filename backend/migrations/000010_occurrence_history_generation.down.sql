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

ALTER TABLE habit_assignments DROP CONSTRAINT assignments_supersedes_family_fk;
ALTER TABLE habit_assignments ADD CONSTRAINT habit_assignments_supersedes_assignment_id_fkey FOREIGN KEY (supersedes_assignment_id) REFERENCES habit_assignments(id) ON DELETE RESTRICT;
ALTER TABLE occurrences DROP COLUMN color_snapshot, DROP COLUMN icon_snapshot, DROP COLUMN description_snapshot;
