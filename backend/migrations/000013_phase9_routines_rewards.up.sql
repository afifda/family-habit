-- Rebuild instead of ALTER TYPE ADD VALUE because every migration is applied
-- atomically and PostgreSQL cannot use a freshly added enum value in that same
-- transaction.
CREATE TYPE ledger_kind_phase9 AS ENUM ('award','approval_reversal','manual_correction','reward_redemption','reward_refund');
DROP TRIGGER IF EXISTS point_ledger_link_guard ON point_ledger;
DROP FUNCTION IF EXISTS validate_ledger_links();
DROP FUNCTION IF EXISTS validate_ledger_reversal();
DROP INDEX IF EXISTS point_ledger_one_award;
DROP INDEX IF EXISTS point_ledger_one_reversal;
DROP INDEX IF EXISTS point_ledger_one_award_attempt;
DROP INDEX IF EXISTS point_ledger_one_reversal_source;
ALTER TABLE point_ledger DROP CONSTRAINT ledger_kind_shape;
ALTER TABLE point_ledger ALTER COLUMN kind TYPE ledger_kind_phase9 USING kind::text::ledger_kind_phase9;
DROP TYPE ledger_kind;
ALTER TYPE ledger_kind_phase9 RENAME TO ledger_kind;
CREATE UNIQUE INDEX point_ledger_one_award ON point_ledger(occurrence_id) WHERE kind='award';
CREATE UNIQUE INDEX point_ledger_one_reversal_source ON point_ledger(reverses_entry_id) WHERE reverses_entry_id IS NOT NULL;
CREATE UNIQUE INDEX point_ledger_one_award_attempt ON point_ledger(completion_attempt_id) WHERE kind='award';

ALTER TABLE families ADD COLUMN rewards_enabled boolean NOT NULL DEFAULT false;

CREATE TABLE routine_groups (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  name text NOT NULL,
  icon text,
  color text,
  starts_at_local time,
  ends_at_local time,
  sort_order integer NOT NULL DEFAULT 0,
  archived_at timestamptz,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT routine_groups_name_valid CHECK (btrim(name) <> '' AND char_length(name) <= 60),
  CONSTRAINT routine_groups_icon_valid CHECK (icon IS NULL OR char_length(icon) <= 40),
  CONSTRAINT routine_groups_color_valid CHECK (color IS NULL OR color ~ '^#[0-9A-Fa-f]{6}$'),
  CONSTRAINT routine_groups_order_valid CHECK (sort_order >= 0),
  CONSTRAINT routine_groups_window_valid CHECK ((starts_at_local IS NULL AND ends_at_local IS NULL) OR (starts_at_local IS NOT NULL AND ends_at_local IS NOT NULL AND starts_at_local <> ends_at_local)),
  CONSTRAINT routine_groups_version_valid CHECK (version > 0),
  UNIQUE (id, family_id)
);
CREATE UNIQUE INDEX routine_groups_active_name ON routine_groups(family_id, lower(btrim(name))) WHERE archived_at IS NULL;
CREATE INDEX routine_groups_active_order ON routine_groups(family_id, sort_order, id) WHERE archived_at IS NULL;

ALTER TABLE habit_assignments
  ADD COLUMN routine_group_id uuid,
  ADD COLUMN sort_order integer NOT NULL DEFAULT 0,
  ADD CONSTRAINT assignments_routine_group_fk FOREIGN KEY (routine_group_id, family_id) REFERENCES routine_groups(id, family_id) ON DELETE RESTRICT,
  ADD CONSTRAINT assignments_sort_order_valid CHECK (sort_order >= 0);
ALTER TABLE one_off_tasks
  ADD COLUMN routine_group_id uuid,
  ADD COLUMN sort_order integer NOT NULL DEFAULT 0,
  ADD CONSTRAINT tasks_routine_group_fk FOREIGN KEY (routine_group_id, family_id) REFERENCES routine_groups(id, family_id) ON DELETE RESTRICT,
  ADD CONSTRAINT tasks_sort_order_valid CHECK (sort_order >= 0);
ALTER TABLE occurrences
  ADD COLUMN routine_group_id_snapshot uuid,
  ADD COLUMN routine_group_name_snapshot text,
  ADD COLUMN routine_group_icon_snapshot text,
  ADD COLUMN routine_group_color_snapshot text,
  ADD COLUMN routine_group_sort_order_snapshot integer,
  ADD COLUMN item_sort_order_snapshot integer NOT NULL DEFAULT 0,
  ADD CONSTRAINT occurrences_item_order_valid CHECK (item_sort_order_snapshot >= 0),
  ADD CONSTRAINT occurrences_routine_snapshot_shape CHECK (
    (routine_group_id_snapshot IS NULL AND routine_group_name_snapshot IS NULL AND routine_group_icon_snapshot IS NULL AND routine_group_color_snapshot IS NULL AND routine_group_sort_order_snapshot IS NULL) OR
    (routine_group_id_snapshot IS NOT NULL AND btrim(coalesce(routine_group_name_snapshot,'')) <> '' AND routine_group_sort_order_snapshot IS NOT NULL)
  );

CREATE TYPE reward_redemption_state AS ENUM ('requested','fulfilled','cancelled');
CREATE TABLE rewards (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  title text NOT NULL,
  description text,
  icon text,
  cost_points integer NOT NULL,
  availability_scope text NOT NULL DEFAULT 'all_active_children',
  archived_at timestamptz,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT rewards_title_valid CHECK (btrim(title) <> '' AND char_length(title) <= 80),
  CONSTRAINT rewards_description_valid CHECK (description IS NULL OR char_length(description) <= 500),
  CONSTRAINT rewards_icon_valid CHECK (icon IS NULL OR char_length(icon) <= 40),
  CONSTRAINT rewards_cost_valid CHECK (cost_points BETWEEN 1 AND 10000),
  CONSTRAINT rewards_availability_scope_valid CHECK (availability_scope IN ('all_active_children','selected_children')),
  CONSTRAINT rewards_version_valid CHECK (version > 0),
  UNIQUE (id, family_id)
);
CREATE INDEX rewards_active_catalog ON rewards(family_id, lower(title), id) WHERE archived_at IS NULL;
CREATE TABLE reward_child_availability (
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  reward_id uuid NOT NULL,
  child_id uuid NOT NULL,
  PRIMARY KEY (reward_id, child_id),
  FOREIGN KEY (reward_id, family_id) REFERENCES rewards(id, family_id) ON DELETE CASCADE,
  FOREIGN KEY (child_id, family_id) REFERENCES children(id, family_id) ON DELETE RESTRICT
);
CREATE TABLE reward_redemptions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  child_id uuid NOT NULL,
  reward_id uuid NOT NULL,
  reward_title_snapshot text NOT NULL,
  reward_icon_snapshot text,
  cost_points_snapshot integer NOT NULL,
  state reward_redemption_state NOT NULL DEFAULT 'requested',
  requested_by_child_id uuid NOT NULL,
  requested_at timestamptz NOT NULL DEFAULT now(),
  decided_by_user_id uuid REFERENCES users(id) ON DELETE RESTRICT,
  decided_at timestamptz,
  cancellation_reason text,
  debit_ledger_entry_id uuid,
  refund_ledger_entry_id uuid,
  version bigint NOT NULL DEFAULT 1,
  CONSTRAINT redemptions_child_scope_fk FOREIGN KEY (child_id, family_id) REFERENCES children(id, family_id) ON DELETE RESTRICT,
  CONSTRAINT redemptions_actor_child_scope_fk FOREIGN KEY (requested_by_child_id, family_id) REFERENCES children(id, family_id) ON DELETE RESTRICT,
  CONSTRAINT redemptions_reward_scope_fk FOREIGN KEY (reward_id, family_id) REFERENCES rewards(id, family_id) ON DELETE RESTRICT,
  CONSTRAINT redemptions_cost_valid CHECK (cost_points_snapshot > 0),
  CONSTRAINT redemptions_actor_matches CHECK (requested_by_child_id = child_id),
  CONSTRAINT redemptions_state_shape CHECK (
    (state='requested' AND decided_by_user_id IS NULL AND decided_at IS NULL AND cancellation_reason IS NULL AND refund_ledger_entry_id IS NULL) OR
    (state='fulfilled' AND decided_by_user_id IS NOT NULL AND decided_at IS NOT NULL AND cancellation_reason IS NULL AND refund_ledger_entry_id IS NULL) OR
    (state='cancelled' AND decided_by_user_id IS NOT NULL AND decided_at IS NOT NULL AND btrim(coalesce(cancellation_reason,'')) <> '' AND refund_ledger_entry_id IS NOT NULL)
  ),
  UNIQUE (id, family_id, child_id),
  UNIQUE (debit_ledger_entry_id),
  UNIQUE (refund_ledger_entry_id)
);
ALTER TABLE point_ledger
  ADD COLUMN reward_redemption_id uuid,
  ADD COLUMN actor_child_id uuid,
  ALTER COLUMN actor_user_id DROP NOT NULL,
  ADD CONSTRAINT ledger_actor_child_scope_fk FOREIGN KEY (actor_child_id, family_id) REFERENCES children(id, family_id) ON DELETE RESTRICT,
  ADD CONSTRAINT ledger_actor_user_scope_fk FOREIGN KEY (family_id, actor_user_id) REFERENCES family_memberships(family_id, user_id) ON DELETE RESTRICT,
  ADD CONSTRAINT ledger_exactly_one_actor CHECK ((actor_user_id IS NULL) <> (actor_child_id IS NULL)),
  ADD CONSTRAINT ledger_redemption_scope_fk FOREIGN KEY (reward_redemption_id, family_id, child_id) REFERENCES reward_redemptions(id, family_id, child_id) DEFERRABLE INITIALLY DEFERRED,
  ADD CONSTRAINT ledger_kind_shape CHECK (
    (kind='award' AND occurrence_id IS NOT NULL AND completion_attempt_id IS NOT NULL AND reverses_entry_id IS NULL AND reward_redemption_id IS NULL AND amount > 0 AND actor_user_id IS NOT NULL) OR
    (kind='approval_reversal' AND occurrence_id IS NOT NULL AND completion_attempt_id IS NULL AND reverses_entry_id IS NOT NULL AND reward_redemption_id IS NULL AND amount < 0 AND actor_user_id IS NOT NULL) OR
    (kind='manual_correction' AND occurrence_id IS NULL AND completion_attempt_id IS NULL AND reverses_entry_id IS NULL AND reward_redemption_id IS NULL AND amount > 0 AND actor_user_id IS NOT NULL AND btrim(coalesce(reason,'')) <> '') OR
    (kind='reward_redemption' AND occurrence_id IS NULL AND completion_attempt_id IS NULL AND reverses_entry_id IS NULL AND reward_redemption_id IS NOT NULL AND amount < 0 AND actor_child_id IS NOT NULL AND reason IS NULL) OR
    (kind='reward_refund' AND occurrence_id IS NULL AND completion_attempt_id IS NULL AND reverses_entry_id IS NOT NULL AND reward_redemption_id IS NOT NULL AND amount > 0 AND actor_user_id IS NOT NULL AND btrim(coalesce(reason,'')) <> '')
  );
ALTER TABLE reward_redemptions
  ADD CONSTRAINT redemptions_debit_fk FOREIGN KEY (debit_ledger_entry_id) REFERENCES point_ledger(id) DEFERRABLE INITIALLY DEFERRED,
  ADD CONSTRAINT redemptions_refund_fk FOREIGN KEY (refund_ledger_entry_id) REFERENCES point_ledger(id) DEFERRABLE INITIALLY DEFERRED;
CREATE UNIQUE INDEX point_ledger_one_reward_debit ON point_ledger(reward_redemption_id) WHERE kind='reward_redemption';
CREATE UNIQUE INDEX point_ledger_one_reward_refund ON point_ledger(reward_redemption_id) WHERE kind='reward_refund';
CREATE INDEX reward_redemptions_queue ON reward_redemptions(family_id,state,requested_at,id);

CREATE OR REPLACE FUNCTION validate_ledger_links() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE original point_ledger%ROWTYPE; attempt completion_attempts%ROWTYPE; occurrence occurrences%ROWTYPE; redemption reward_redemptions%ROWTYPE;
BEGIN
  IF NEW.kind='award' THEN
    SELECT * INTO attempt FROM completion_attempts WHERE id=NEW.completion_attempt_id FOR KEY SHARE;
    SELECT * INTO occurrence FROM occurrences WHERE id=NEW.occurrence_id FOR KEY SHARE;
    IF NOT FOUND OR attempt.occurrence_id<>NEW.occurrence_id OR attempt.family_id<>NEW.family_id OR attempt.child_id<>NEW.child_id OR attempt.decision<>'approved' OR occurrence.family_id<>NEW.family_id OR occurrence.child_id<>NEW.child_id OR occurrence.points_snapshot<>NEW.amount THEN RAISE EXCEPTION 'award must match its approved attempt and occurrence snapshot'; END IF;
  ELSIF NEW.kind='approval_reversal' THEN
    SELECT * INTO original FROM point_ledger WHERE id=NEW.reverses_entry_id FOR KEY SHARE;
    IF NOT FOUND OR original.kind<>'award' OR original.family_id<>NEW.family_id OR original.child_id<>NEW.child_id OR original.occurrence_id<>NEW.occurrence_id OR original.amount<>-NEW.amount THEN RAISE EXCEPTION 'approval reversal must exactly negate its matching award'; END IF;
  ELSIF NEW.kind='reward_redemption' THEN
    SELECT * INTO redemption FROM reward_redemptions WHERE id=NEW.reward_redemption_id;
    IF NOT FOUND OR redemption.family_id<>NEW.family_id OR redemption.child_id<>NEW.child_id OR NEW.actor_child_id<>NEW.child_id OR NEW.amount<>-redemption.cost_points_snapshot THEN RAISE EXCEPTION 'reward debit must match redemption'; END IF;
  ELSIF NEW.kind='reward_refund' THEN
    SELECT * INTO original FROM point_ledger WHERE id=NEW.reverses_entry_id FOR KEY SHARE;
    SELECT * INTO redemption FROM reward_redemptions WHERE id=NEW.reward_redemption_id;
    IF original.id IS NULL OR redemption.id IS NULL OR original.kind<>'reward_redemption' OR original.reward_redemption_id<>NEW.reward_redemption_id OR original.family_id<>NEW.family_id OR original.child_id<>NEW.child_id OR original.amount<>-NEW.amount OR redemption.family_id<>NEW.family_id OR redemption.child_id<>NEW.child_id THEN RAISE EXCEPTION 'reward refund must exactly negate debit'; END IF;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER point_ledger_link_guard BEFORE INSERT ON point_ledger FOR EACH ROW EXECUTE FUNCTION validate_ledger_links();

CREATE OR REPLACE FUNCTION prevent_occurrence_snapshot_rewrite() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (OLD.title_snapshot IS DISTINCT FROM NEW.title_snapshot OR OLD.description_snapshot IS DISTINCT FROM NEW.description_snapshot OR OLD.icon_snapshot IS DISTINCT FROM NEW.icon_snapshot OR OLD.color_snapshot IS DISTINCT FROM NEW.color_snapshot OR OLD.points_snapshot IS DISTINCT FROM NEW.points_snapshot OR OLD.item_type_snapshot IS DISTINCT FROM NEW.item_type_snapshot OR OLD.assignment_id IS DISTINCT FROM NEW.assignment_id OR OLD.task_id IS DISTINCT FROM NEW.task_id OR OLD.source_type IS DISTINCT FROM NEW.source_type OR OLD.child_id IS DISTINCT FROM NEW.child_id OR OLD.family_id IS DISTINCT FROM NEW.family_id OR OLD.local_date IS DISTINCT FROM NEW.local_date OR OLD.due_date IS DISTINCT FROM NEW.due_date OR OLD.routine_group_id_snapshot IS DISTINCT FROM NEW.routine_group_id_snapshot OR OLD.routine_group_name_snapshot IS DISTINCT FROM NEW.routine_group_name_snapshot OR OLD.routine_group_icon_snapshot IS DISTINCT FROM NEW.routine_group_icon_snapshot OR OLD.routine_group_color_snapshot IS DISTINCT FROM NEW.routine_group_color_snapshot OR OLD.routine_group_sort_order_snapshot IS DISTINCT FROM NEW.routine_group_sort_order_snapshot OR OLD.item_sort_order_snapshot IS DISTINCT FROM NEW.item_sort_order_snapshot) AND NOT (OLD.source_type='task' AND OLD.state='not_started' AND NOT EXISTS (SELECT 1 FROM completion_attempts WHERE occurrence_id=OLD.id)) THEN RAISE EXCEPTION 'occurrence identity and snapshots are immutable'; END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION validate_routine_group_active() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.routine_group_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM routine_groups WHERE id=NEW.routine_group_id AND family_id=NEW.family_id AND archived_at IS NULL) THEN RAISE EXCEPTION 'routine group must be active in household'; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER assignments_active_group BEFORE INSERT OR UPDATE OF routine_group_id ON habit_assignments FOR EACH ROW EXECUTE FUNCTION validate_routine_group_active();
CREATE TRIGGER tasks_active_group BEFORE INSERT OR UPDATE OF routine_group_id ON one_off_tasks FOR EACH ROW EXECUTE FUNCTION validate_routine_group_active();
