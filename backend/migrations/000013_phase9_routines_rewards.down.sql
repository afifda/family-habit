DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM point_ledger WHERE kind::text IN ('reward_redemption','reward_refund')) OR EXISTS(SELECT 1 FROM reward_redemptions) THEN
    RAISE EXCEPTION 'cannot roll back phase 9 while reward history exists';
  END IF;
END $$;
DROP TRIGGER IF EXISTS point_ledger_link_guard ON point_ledger;
DROP FUNCTION IF EXISTS validate_ledger_links();
DROP TRIGGER IF EXISTS tasks_active_group ON one_off_tasks;
DROP TRIGGER IF EXISTS assignments_active_group ON habit_assignments;
DROP FUNCTION IF EXISTS validate_routine_group_active();
DROP INDEX IF EXISTS point_ledger_one_reward_refund;
DROP INDEX IF EXISTS point_ledger_one_reward_debit;
ALTER TABLE reward_redemptions DROP CONSTRAINT IF EXISTS redemptions_refund_fk, DROP CONSTRAINT IF EXISTS redemptions_debit_fk;
ALTER TABLE point_ledger DROP CONSTRAINT IF EXISTS ledger_kind_shape, DROP CONSTRAINT IF EXISTS ledger_redemption_scope_fk, DROP CONSTRAINT IF EXISTS ledger_actor_child_scope_fk, DROP CONSTRAINT IF EXISTS ledger_actor_user_scope_fk, DROP CONSTRAINT IF EXISTS ledger_exactly_one_actor, DROP COLUMN IF EXISTS reward_redemption_id, DROP COLUMN IF EXISTS actor_child_id, ALTER COLUMN actor_user_id SET NOT NULL;
ALTER TABLE point_ledger ADD CONSTRAINT ledger_kind_shape CHECK ((kind='award' AND occurrence_id IS NOT NULL AND completion_attempt_id IS NOT NULL AND reverses_entry_id IS NULL AND amount>0) OR (kind='approval_reversal' AND occurrence_id IS NOT NULL AND completion_attempt_id IS NULL AND reverses_entry_id IS NOT NULL AND amount<0) OR (kind='manual_correction' AND occurrence_id IS NULL AND completion_attempt_id IS NULL AND reverses_entry_id IS NULL AND amount>0 AND btrim(coalesce(reason,''))<>''));
DROP TABLE IF EXISTS reward_redemptions, reward_child_availability, rewards;
DROP TYPE IF EXISTS reward_redemption_state;
ALTER TABLE occurrences DROP COLUMN IF EXISTS routine_group_id_snapshot, DROP COLUMN IF EXISTS routine_group_name_snapshot, DROP COLUMN IF EXISTS routine_group_icon_snapshot, DROP COLUMN IF EXISTS routine_group_color_snapshot, DROP COLUMN IF EXISTS routine_group_sort_order_snapshot, DROP COLUMN IF EXISTS item_sort_order_snapshot;
CREATE OR REPLACE FUNCTION prevent_occurrence_snapshot_rewrite() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (OLD.title_snapshot IS DISTINCT FROM NEW.title_snapshot OR OLD.description_snapshot IS DISTINCT FROM NEW.description_snapshot OR OLD.icon_snapshot IS DISTINCT FROM NEW.icon_snapshot OR OLD.color_snapshot IS DISTINCT FROM NEW.color_snapshot OR OLD.points_snapshot IS DISTINCT FROM NEW.points_snapshot OR OLD.item_type_snapshot IS DISTINCT FROM NEW.item_type_snapshot OR OLD.assignment_id IS DISTINCT FROM NEW.assignment_id OR OLD.task_id IS DISTINCT FROM NEW.task_id OR OLD.source_type IS DISTINCT FROM NEW.source_type OR OLD.child_id IS DISTINCT FROM NEW.child_id OR OLD.family_id IS DISTINCT FROM NEW.family_id OR OLD.local_date IS DISTINCT FROM NEW.local_date OR OLD.due_date IS DISTINCT FROM NEW.due_date) AND NOT (OLD.source_type='task' AND OLD.state='not_started' AND NOT EXISTS (SELECT 1 FROM completion_attempts WHERE occurrence_id=OLD.id)) THEN RAISE EXCEPTION 'occurrence identity and snapshots are immutable'; END IF;
  RETURN NEW;
END $$;
ALTER TABLE one_off_tasks DROP CONSTRAINT IF EXISTS tasks_routine_group_fk, DROP COLUMN IF EXISTS routine_group_id, DROP COLUMN IF EXISTS sort_order;
ALTER TABLE habit_assignments DROP CONSTRAINT IF EXISTS assignments_routine_group_fk, DROP COLUMN IF EXISTS routine_group_id, DROP COLUMN IF EXISTS sort_order;
DROP TABLE IF EXISTS routine_groups;
ALTER TABLE families DROP COLUMN IF EXISTS rewards_enabled;
-- PostgreSQL enum values are intentionally retained on rollback.

CREATE OR REPLACE FUNCTION validate_ledger_links() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE original point_ledger%ROWTYPE; attempt completion_attempts%ROWTYPE; occurrence occurrences%ROWTYPE;
BEGIN
  IF NEW.kind='award' THEN SELECT * INTO attempt FROM completion_attempts WHERE id=NEW.completion_attempt_id FOR KEY SHARE; SELECT * INTO occurrence FROM occurrences WHERE id=NEW.occurrence_id FOR KEY SHARE; IF NOT FOUND OR attempt.occurrence_id<>NEW.occurrence_id OR attempt.family_id<>NEW.family_id OR attempt.child_id<>NEW.child_id OR attempt.decision<>'approved' OR occurrence.family_id<>NEW.family_id OR occurrence.child_id<>NEW.child_id OR occurrence.points_snapshot<>NEW.amount THEN RAISE EXCEPTION 'award must match its approved attempt and occurrence snapshot'; END IF;
  ELSIF NEW.kind='approval_reversal' THEN SELECT * INTO original FROM point_ledger WHERE id=NEW.reverses_entry_id FOR KEY SHARE; IF NOT FOUND OR original.kind<>'award' OR original.family_id<>NEW.family_id OR original.child_id<>NEW.child_id OR original.occurrence_id<>NEW.occurrence_id OR original.amount<>-NEW.amount THEN RAISE EXCEPTION 'approval reversal must exactly negate its matching award'; END IF; END IF; RETURN NEW;
END $$;
CREATE TRIGGER point_ledger_link_guard BEFORE INSERT ON point_ledger FOR EACH ROW EXECUTE FUNCTION validate_ledger_links();
