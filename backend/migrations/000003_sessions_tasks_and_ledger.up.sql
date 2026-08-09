ALTER TABLE families
  ALTER COLUMN timezone SET DEFAULT 'Asia/Jakarta',
  ALTER COLUMN week_starts_on SET DEFAULT 0,
  ADD COLUMN parent_idle_minutes smallint NOT NULL DEFAULT 15,
  ADD CONSTRAINT families_parent_idle_valid CHECK (parent_idle_minutes IN (5, 15, 30));

ALTER TYPE occurrence_state ADD VALUE IF NOT EXISTS 'cancelled';
ALTER TYPE completion_decision ADD VALUE IF NOT EXISTS 'cancelled';
ALTER TYPE ledger_kind RENAME VALUE 'reversal' TO 'approval_reversal';
ALTER TYPE ledger_kind RENAME VALUE 'correction' TO 'manual_correction';

CREATE TYPE session_mode AS ENUM ('shared', 'parent', 'child');
CREATE TYPE task_state AS ENUM ('active', 'cancelled');

CREATE TABLE sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  active_child_id uuid,
  token_hash bytea NOT NULL UNIQUE,
  mode session_mode NOT NULL DEFAULT 'shared',
  parent_unlocked_at timestamptz,
  last_activity_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sessions_membership_fk FOREIGN KEY (family_id, user_id)
    REFERENCES family_memberships(family_id, user_id) ON DELETE RESTRICT,
  CONSTRAINT sessions_child_family_fk FOREIGN KEY (active_child_id, family_id)
    REFERENCES children(id, family_id) ON DELETE RESTRICT,
  CONSTRAINT sessions_expiry_valid CHECK (expires_at > created_at),
  CONSTRAINT sessions_mode_shape CHECK (
    (mode = 'shared' AND active_child_id IS NULL AND parent_unlocked_at IS NULL) OR
    (mode = 'parent' AND active_child_id IS NULL AND parent_unlocked_at IS NOT NULL) OR
    (mode = 'child' AND active_child_id IS NOT NULL AND parent_unlocked_at IS NULL)
  )
);
CREATE INDEX sessions_active_user ON sessions (family_id, user_id, expires_at)
  WHERE revoked_at IS NULL;

CREATE TABLE one_off_tasks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  child_id uuid NOT NULL,
  title text NOT NULL,
  description text,
  icon text,
  color text,
  points integer NOT NULL,
  due_date date NOT NULL,
  state task_state NOT NULL DEFAULT 'active',
  cancellation_reason text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT tasks_child_family_fk FOREIGN KEY (child_id, family_id)
    REFERENCES children(id, family_id) ON DELETE RESTRICT,
  CONSTRAINT tasks_title_not_blank CHECK (btrim(title) <> ''),
  CONSTRAINT tasks_points_valid CHECK (points BETWEEN 1 AND 10000),
  CONSTRAINT tasks_state_shape CHECK (
    (state = 'active' AND cancellation_reason IS NULL) OR
    (state = 'cancelled' AND btrim(coalesce(cancellation_reason, '')) <> '')
  ),
  UNIQUE (id, family_id, child_id)
);

ALTER TABLE occurrences
  ALTER COLUMN assignment_id DROP NOT NULL,
  ADD COLUMN task_id uuid,
  ADD COLUMN source_type text NOT NULL DEFAULT 'habit',
  ADD COLUMN due_date date,
  ADD COLUMN item_type_snapshot text NOT NULL DEFAULT 'habit',
  ADD COLUMN version bigint NOT NULL DEFAULT 1,
  ADD CONSTRAINT occurrences_task_scope_fk FOREIGN KEY (task_id, family_id, child_id)
    REFERENCES one_off_tasks(id, family_id, child_id) ON DELETE RESTRICT,
  ADD CONSTRAINT occurrences_source_shape CHECK (
    (source_type = 'habit' AND assignment_id IS NOT NULL AND task_id IS NULL AND item_type_snapshot = 'habit') OR
    (source_type = 'task' AND assignment_id IS NULL AND task_id IS NOT NULL AND due_date IS NOT NULL AND item_type_snapshot = 'task')
  ),
  ADD CONSTRAINT occurrences_version_positive CHECK (version > 0);
CREATE UNIQUE INDEX occurrences_one_per_task ON occurrences (family_id, task_id)
  WHERE task_id IS NOT NULL;

ALTER TABLE habit_schedules DROP CONSTRAINT schedules_shape_valid;
ALTER TABLE habit_schedules ADD CONSTRAINT schedules_shape_valid CHECK (
  (kind = 'daily' AND cardinality(weekdays) = 0 AND due_date IS NULL) OR
  (kind = 'weekdays' AND cardinality(weekdays) BETWEEN 1 AND 7 AND due_date IS NULL)
);

ALTER TABLE completion_attempts
  ADD CONSTRAINT attempts_id_scope_unique UNIQUE (id, family_id, child_id);

ALTER TABLE point_ledger
  ADD COLUMN reverses_entry_id uuid REFERENCES point_ledger(id) ON DELETE RESTRICT,
  DROP CONSTRAINT ledger_kind_shape,
  ADD CONSTRAINT ledger_attempt_scope_fk FOREIGN KEY (completion_attempt_id, family_id, child_id)
    REFERENCES completion_attempts(id, family_id, child_id) ON DELETE RESTRICT,
  ADD CONSTRAINT ledger_kind_shape CHECK (
    (kind = 'award' AND occurrence_id IS NOT NULL AND completion_attempt_id IS NOT NULL AND reverses_entry_id IS NULL AND points > 0) OR
    (kind = 'approval_reversal' AND occurrence_id IS NOT NULL AND completion_attempt_id IS NULL AND reverses_entry_id IS NOT NULL AND points < 0) OR
    (kind = 'manual_correction' AND occurrence_id IS NULL AND completion_attempt_id IS NULL AND reverses_entry_id IS NULL AND points > 0 AND btrim(coalesce(reason, '')) <> '')
  );
CREATE UNIQUE INDEX point_ledger_one_reversal_source ON point_ledger (reverses_entry_id)
  WHERE reverses_entry_id IS NOT NULL;

CREATE FUNCTION validate_ledger_reversal() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE original point_ledger%ROWTYPE;
BEGIN
  IF NEW.kind <> 'approval_reversal' THEN
    RETURN NEW;
  END IF;
  SELECT * INTO original FROM point_ledger WHERE id = NEW.reverses_entry_id FOR KEY SHARE;
  IF NOT FOUND OR original.kind <> 'award' OR
     original.family_id <> NEW.family_id OR original.child_id <> NEW.child_id OR
     original.occurrence_id <> NEW.occurrence_id OR original.points <> -NEW.points THEN
    RAISE EXCEPTION 'approval reversal must exactly negate its matching award';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER point_ledger_reversal_guard
  BEFORE INSERT ON point_ledger FOR EACH ROW EXECUTE FUNCTION validate_ledger_reversal();

ALTER TABLE audit_events
  ALTER COLUMN family_id SET NOT NULL,
  ADD COLUMN session_id uuid REFERENCES sessions(id) ON DELETE RESTRICT,
  ADD COLUMN before_status text,
  ADD COLUMN after_status text,
  ADD COLUMN idempotency_key text,
  DROP CONSTRAINT audit_actor_present,
  ADD CONSTRAINT audit_exactly_one_actor CHECK ((actor_user_id IS NULL) <> (actor_child_id IS NULL));

CREATE TABLE idempotency_records (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE RESTRICT,
  route_family text NOT NULL,
  idempotency_key text NOT NULL,
  request_hash bytea NOT NULL,
  response_status smallint,
  response_body jsonb,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT idempotency_route_not_blank CHECK (btrim(route_family) <> ''),
  CONSTRAINT idempotency_key_not_blank CHECK (btrim(idempotency_key) <> ''),
  CONSTRAINT idempotency_status_valid CHECK (response_status IS NULL OR response_status BETWEEN 200 AND 599),
  CONSTRAINT idempotency_expiry_valid CHECK (expires_at > created_at),
  UNIQUE (family_id, session_id, route_family, idempotency_key)
);
CREATE INDEX idempotency_expiry ON idempotency_records (expires_at);

