-- Phase 9 release invariants discovered by the strict release audit.
ALTER TABLE families ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0);
ALTER TABLE routine_groups ADD CONSTRAINT routine_groups_icon_safe CHECK(icon IS NULL OR icon IN ('🌅','☀️','🏫','🌆','🌙','⭐','🎁','🍦','🎮','🎬','📚','🚲','🍕','🎨','⚽'));
ALTER TABLE rewards ADD CONSTRAINT rewards_icon_safe CHECK(icon IS NULL OR icon IN ('🌅','☀️','🏫','🌆','🌙','⭐','🎁','🍦','🎮','🎬','📚','🚲','🍕','🎨','⚽'));
ALTER TABLE reward_redemptions
  ADD CONSTRAINT redemptions_decider_household_fk
  FOREIGN KEY (family_id, decided_by_user_id)
  REFERENCES family_memberships(family_id, user_id) ON DELETE RESTRICT;

CREATE OR REPLACE FUNCTION validate_reward_ledger_commit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r reward_redemptions%ROWTYPE;
BEGIN
  SELECT * INTO r FROM reward_redemptions WHERE id=NEW.reward_redemption_id;
  IF NEW.kind='reward_redemption' AND
     (r.id IS NULL OR r.state<>'requested' OR r.debit_ledger_entry_id<>NEW.id OR
      NEW.actor_child_id<>r.requested_by_child_id) THEN
    RAISE EXCEPTION 'reward debit requires its requested redemption and child actor';
  ELSIF NEW.kind='reward_refund' AND
     (r.id IS NULL OR r.state<>'cancelled' OR r.refund_ledger_entry_id<>NEW.id OR
      r.decided_by_user_id IS NULL OR NEW.actor_user_id<>r.decided_by_user_id OR
      btrim(NEW.reason)<>btrim(r.cancellation_reason)) THEN
    RAISE EXCEPTION 'reward refund requires its cancelled redemption, deciding parent, and exact reason';
  END IF;
  RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER reward_ledger_commit_guard
AFTER INSERT ON point_ledger DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW WHEN (NEW.kind IN ('reward_redemption','reward_refund'))
EXECUTE FUNCTION validate_reward_ledger_commit();

CREATE OR REPLACE FUNCTION validate_selected_reward_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.availability_scope='selected_children'
     AND NOT EXISTS(SELECT 1 FROM reward_child_availability a WHERE a.reward_id=NEW.id) THEN
    RAISE EXCEPTION 'selected child reward requires at least one eligible child';
  END IF;
  RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER rewards_selected_scope_guard
AFTER INSERT OR UPDATE OF availability_scope ON rewards DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_selected_reward_scope();
CREATE OR REPLACE FUNCTION validate_removed_reward_availability() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF EXISTS(SELECT 1 FROM rewards r WHERE r.id=OLD.reward_id AND r.availability_scope='selected_children')
     AND NOT EXISTS(SELECT 1 FROM reward_child_availability a WHERE a.reward_id=OLD.reward_id) THEN
    RAISE EXCEPTION 'selected child reward requires at least one eligible child';
  END IF;
  RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER reward_availability_selected_scope_guard
AFTER DELETE OR UPDATE OF reward_id ON reward_child_availability DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_removed_reward_availability();

WITH ranked AS (
  SELECT id,row_number() OVER(PARTITION BY family_id ORDER BY sort_order,id)-1 AS n
  FROM routine_groups WHERE archived_at IS NULL
)
UPDATE routine_groups g SET sort_order=ranked.n FROM ranked WHERE g.id=ranked.id;
CREATE UNIQUE INDEX routine_groups_active_sort_order
  ON routine_groups(family_id,sort_order) WHERE archived_at IS NULL;
