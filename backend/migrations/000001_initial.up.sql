CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE membership_role AS ENUM ('owner', 'parent');
CREATE TYPE schedule_kind AS ENUM ('daily', 'weekdays', 'one_off');
CREATE TYPE occurrence_state AS ENUM ('not_started', 'pending_approval', 'approved', 'approval_reversed');
CREATE TYPE completion_decision AS ENUM ('pending', 'withdrawn', 'approved', 'rejected');
CREATE TYPE ledger_kind AS ENUM ('award', 'reversal', 'correction');

CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text NOT NULL,
  password_hash text NOT NULL,
  parent_pin_hash text,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT users_email_not_blank CHECK (btrim(email) <> ''),
  CONSTRAINT users_password_hash_not_blank CHECK (btrim(password_hash) <> '')
);
CREATE UNIQUE INDEX users_email_unique ON users (lower(email));

CREATE TABLE families (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  timezone text NOT NULL DEFAULT 'Europe/Berlin',
  week_starts_on smallint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT families_name_not_blank CHECK (btrim(name) <> ''),
  CONSTRAINT families_timezone_not_blank CHECK (btrim(timezone) <> ''),
  CONSTRAINT families_week_start_valid CHECK (week_starts_on BETWEEN 0 AND 6)
);

CREATE TABLE family_memberships (
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  role membership_role NOT NULL DEFAULT 'parent',
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (family_id, user_id)
);

CREATE TABLE children (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  nickname text NOT NULL,
  avatar text,
  color text,
  pin_hash text,
  archived_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT children_nickname_not_blank CHECK (btrim(nickname) <> '')
);
CREATE UNIQUE INDEX children_active_nickname_unique
  ON children (family_id, lower(nickname)) WHERE archived_at IS NULL;

CREATE TABLE habits (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  title text NOT NULL,
  description text,
  icon text,
  color text,
  archived_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT habits_title_not_blank CHECK (btrim(title) <> '')
);

CREATE TABLE habit_assignments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  habit_id uuid NOT NULL REFERENCES habits(id) ON DELETE RESTRICT,
  child_id uuid NOT NULL REFERENCES children(id) ON DELETE RESTRICT,
  points integer NOT NULL,
  effective_from date NOT NULL,
  effective_until date,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT assignments_points_valid CHECK (points BETWEEN 1 AND 10000),
  CONSTRAINT assignments_date_range_valid CHECK (effective_until IS NULL OR effective_until >= effective_from),
  UNIQUE (id, child_id),
  UNIQUE (id, family_id)
);
CREATE INDEX habit_assignments_child_dates ON habit_assignments (child_id, effective_from, effective_until);

CREATE TABLE habit_schedules (
  assignment_id uuid PRIMARY KEY REFERENCES habit_assignments(id) ON DELETE CASCADE,
  kind schedule_kind NOT NULL,
  weekdays smallint[] NOT NULL DEFAULT '{}',
  due_date date,
  CONSTRAINT schedules_shape_valid CHECK (
    (kind = 'daily' AND cardinality(weekdays) = 0 AND due_date IS NULL) OR
    (kind = 'weekdays' AND cardinality(weekdays) BETWEEN 1 AND 7 AND due_date IS NULL) OR
    (kind = 'one_off' AND cardinality(weekdays) = 0 AND due_date IS NOT NULL)
  ),
  CONSTRAINT schedules_weekdays_valid CHECK (weekdays <@ ARRAY[0,1,2,3,4,5,6]::smallint[])
);

CREATE TABLE occurrences (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  assignment_id uuid NOT NULL,
  child_id uuid NOT NULL REFERENCES children(id) ON DELETE RESTRICT,
  local_date date NOT NULL,
  title_snapshot text NOT NULL,
  points_snapshot integer NOT NULL,
  state occurrence_state NOT NULL DEFAULT 'not_started',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT occurrences_assignment_child_fk FOREIGN KEY (assignment_id, child_id)
    REFERENCES habit_assignments(id, child_id) ON DELETE RESTRICT,
  CONSTRAINT occurrences_assignment_family_fk FOREIGN KEY (assignment_id, family_id)
    REFERENCES habit_assignments(id, family_id) ON DELETE RESTRICT,
  CONSTRAINT occurrences_title_not_blank CHECK (btrim(title_snapshot) <> ''),
  CONSTRAINT occurrences_points_valid CHECK (points_snapshot BETWEEN 1 AND 10000),
  UNIQUE (assignment_id, child_id, local_date),
  UNIQUE (id, child_id),
  UNIQUE (id, family_id)
);
CREATE INDEX occurrences_today ON occurrences (family_id, child_id, local_date);

CREATE TABLE completion_attempts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  occurrence_id uuid NOT NULL,
  child_id uuid NOT NULL,
  attempt_number integer NOT NULL,
  decision completion_decision NOT NULL DEFAULT 'pending',
  submitted_at timestamptz NOT NULL DEFAULT now(),
  decided_at timestamptz,
  decided_by uuid REFERENCES users(id) ON DELETE RESTRICT,
  decision_note text,
  CONSTRAINT attempts_occurrence_child_fk FOREIGN KEY (occurrence_id, child_id)
    REFERENCES occurrences(id, child_id) ON DELETE RESTRICT,
  CONSTRAINT attempts_occurrence_family_fk FOREIGN KEY (occurrence_id, family_id)
    REFERENCES occurrences(id, family_id) ON DELETE RESTRICT,
  CONSTRAINT attempts_number_positive CHECK (attempt_number > 0),
  CONSTRAINT attempts_decision_shape CHECK (
    (decision = 'pending' AND decided_at IS NULL AND decided_by IS NULL) OR
    (decision = 'withdrawn' AND decided_at IS NOT NULL AND decided_by IS NULL) OR
    (decision IN ('approved', 'rejected') AND decided_at IS NOT NULL AND decided_by IS NOT NULL)
  ),
  UNIQUE (occurrence_id, attempt_number)
);
CREATE UNIQUE INDEX completion_attempts_one_pending
  ON completion_attempts (occurrence_id) WHERE decision = 'pending';

CREATE TABLE point_ledger (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  child_id uuid NOT NULL REFERENCES children(id) ON DELETE RESTRICT,
  occurrence_id uuid,
  completion_attempt_id uuid REFERENCES completion_attempts(id) ON DELETE RESTRICT,
  kind ledger_kind NOT NULL,
  points integer NOT NULL,
  reason text,
  actor_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ledger_occurrence_family_fk FOREIGN KEY (occurrence_id, family_id)
    REFERENCES occurrences(id, family_id) ON DELETE RESTRICT,
  CONSTRAINT ledger_nonzero CHECK (points <> 0),
  CONSTRAINT ledger_kind_shape CHECK (
    (kind = 'award' AND occurrence_id IS NOT NULL AND completion_attempt_id IS NOT NULL AND points > 0) OR
    (kind = 'reversal' AND occurrence_id IS NOT NULL AND completion_attempt_id IS NULL AND points < 0) OR
    (kind = 'correction' AND occurrence_id IS NULL AND completion_attempt_id IS NULL AND btrim(coalesce(reason, '')) <> '')
  )
);
CREATE UNIQUE INDEX point_ledger_one_award ON point_ledger (occurrence_id) WHERE kind = 'award';
CREATE UNIQUE INDEX point_ledger_one_reversal ON point_ledger (occurrence_id) WHERE kind = 'reversal';
CREATE INDEX point_ledger_child_history ON point_ledger (family_id, child_id, created_at DESC);

CREATE TABLE audit_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid REFERENCES families(id) ON DELETE RESTRICT,
  actor_user_id uuid REFERENCES users(id) ON DELETE RESTRICT,
  actor_child_id uuid REFERENCES children(id) ON DELETE RESTRICT,
  action text NOT NULL,
  subject_type text,
  subject_id uuid,
  metadata jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT audit_action_not_blank CHECK (btrim(action) <> ''),
  CONSTRAINT audit_metadata_object CHECK (jsonb_typeof(metadata) = 'object'),
  CONSTRAINT audit_actor_present CHECK (actor_user_id IS NOT NULL OR actor_child_id IS NOT NULL)
);
CREATE INDEX audit_events_family_created ON audit_events (family_id, created_at DESC);
