CREATE TYPE reward_collection_period AS ENUM ('daily','weekly','monthly');

CREATE TABLE reward_eligibility_policies (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  enabled boolean NOT NULL,
  collection_period reward_collection_period NOT NULL,
  minimum_points integer NOT NULL,
  minimum_completion_percentage integer,
  maximum_redemptions integer,
  grace_hours integer NOT NULL DEFAULT 24,
  timezone text NOT NULL,
  week_starts_on smallint NOT NULL,
  effective_from date NOT NULL,
  version bigint NOT NULL,
  created_by_user_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT reward_policy_creator_fk FOREIGN KEY(family_id,created_by_user_id) REFERENCES family_memberships(family_id,user_id) ON DELETE RESTRICT,
  CONSTRAINT reward_policy_points_valid CHECK(minimum_points BETWEEN 1 AND 1000000),
  CONSTRAINT reward_policy_completion_valid CHECK(minimum_completion_percentage IS NULL OR minimum_completion_percentage BETWEEN 1 AND 100),
  CONSTRAINT reward_policy_limit_valid CHECK(maximum_redemptions IS NULL OR maximum_redemptions BETWEEN 1 AND 100),
  CONSTRAINT reward_policy_grace_valid CHECK(grace_hours IN (0,12,24,48)),
  CONSTRAINT reward_policy_daily_grace_valid CHECK(collection_period<>'daily' OR grace_hours=0),
  CONSTRAINT reward_policy_week_start_valid CHECK(week_starts_on IN (0,1)),
  CONSTRAINT reward_policy_version_valid CHECK(version > 0),
  UNIQUE(family_id,version),
  UNIQUE(family_id,effective_from),
  UNIQUE(id,family_id)
);
CREATE INDEX reward_policy_effective ON reward_eligibility_policies(family_id,effective_from DESC,version DESC);

CREATE TABLE reward_period_evaluations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE RESTRICT,
  child_id uuid NOT NULL,
  policy_id uuid NOT NULL,
  collection_start date NOT NULL,
  collection_end date NOT NULL,
  evaluated_at timestamptz NOT NULL DEFAULT now(),
  evaluation_cutoff timestamptz NOT NULL,
  eligible boolean NOT NULL,
  eligible_from date NOT NULL,
  eligible_until date NOT NULL,
  points_collected bigint NOT NULL,
  assigned_count integer NOT NULL,
  approved_count integer NOT NULL,
  completion_percentage integer NOT NULL,
  policy_snapshot jsonb NOT NULL,
  CONSTRAINT reward_evaluation_child_fk FOREIGN KEY(child_id,family_id) REFERENCES children(id,family_id) ON DELETE RESTRICT,
  CONSTRAINT reward_evaluation_policy_fk FOREIGN KEY(policy_id,family_id) REFERENCES reward_eligibility_policies(id,family_id) ON DELETE RESTRICT,
  CONSTRAINT reward_evaluation_period_valid CHECK(collection_start<=collection_end AND eligible_from<=eligible_until),
  CONSTRAINT reward_evaluation_counts_valid CHECK(points_collected>=0 AND assigned_count>=0 AND approved_count>=0 AND approved_count<=assigned_count AND completion_percentage BETWEEN 0 AND 100),
  UNIQUE(family_id,child_id,policy_id,collection_start,collection_end),
  UNIQUE(id,family_id,child_id)
);
CREATE INDEX reward_evaluation_child_history ON reward_period_evaluations(family_id,child_id,collection_start DESC,id DESC);

CREATE OR REPLACE FUNCTION validate_reward_evaluation_shape() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p reward_eligibility_policies%ROWTYPE;
DECLARE expected_cutoff timestamptz;
BEGIN
  SELECT * INTO p FROM reward_eligibility_policies WHERE id=NEW.policy_id AND family_id=NEW.family_id;
  IF NOT FOUND OR NEW.collection_start<p.effective_from THEN RAISE EXCEPTION 'invalid reward evaluation policy period'; END IF;
  IF p.collection_period='daily' AND (NEW.collection_end<>NEW.collection_start OR NEW.eligible_from<>NEW.collection_end+1 OR NEW.eligible_until<>NEW.eligible_from) THEN
    RAISE EXCEPTION 'invalid daily reward evaluation window';
  ELSIF p.collection_period='weekly' AND (NEW.collection_end<>NEW.collection_start+6 OR NEW.eligible_from<>NEW.collection_end+1 OR NEW.eligible_until<>NEW.eligible_from+6) THEN
    RAISE EXCEPTION 'invalid weekly reward evaluation window';
  ELSIF p.collection_period='monthly' AND (extract(day from NEW.collection_start)<>1 OR NEW.collection_end<>(NEW.collection_start+interval '1 month'-interval '1 day')::date OR NEW.eligible_from<>NEW.collection_end+1 OR NEW.eligible_until<>(NEW.eligible_from+interval '1 month'-interval '1 day')::date) THEN
    RAISE EXCEPTION 'invalid monthly reward evaluation window';
  END IF;
  expected_cutoff := NEW.eligible_from::timestamp AT TIME ZONE p.timezone;
  expected_cutoff := expected_cutoff + make_interval(hours=>p.grace_hours);
  IF NEW.evaluation_cutoff<>expected_cutoff THEN RAISE EXCEPTION 'invalid reward evaluation cutoff'; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER reward_evaluation_shape BEFORE INSERT ON reward_period_evaluations FOR EACH ROW EXECUTE FUNCTION validate_reward_evaluation_shape();

CREATE TABLE reward_evaluation_rule_results (
  evaluation_id uuid NOT NULL REFERENCES reward_period_evaluations(id) ON DELETE RESTRICT,
  rule_type text NOT NULL,
  target integer NOT NULL,
  actual integer NOT NULL,
  passed boolean NOT NULL,
  PRIMARY KEY(evaluation_id,rule_type),
  CONSTRAINT reward_rule_type_valid CHECK(rule_type IN ('minimum_points','minimum_completion_percentage'))
);

CREATE TABLE reward_evaluation_adjustments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  evaluation_id uuid NOT NULL REFERENCES reward_period_evaluations(id) ON DELETE RESTRICT,
  ledger_entry_id uuid NOT NULL REFERENCES point_ledger(id) ON DELETE RESTRICT,
  points_delta integer NOT NULL,
  approved_count_delta integer NOT NULL,
  reason text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(ledger_entry_id),
  CONSTRAINT reward_adjustment_delta_valid CHECK(points_delta<0 AND approved_count_delta<=0 AND btrim(reason)<>'')
);

ALTER TABLE reward_redemptions
  ADD COLUMN eligibility_evaluation_id uuid,
  ADD CONSTRAINT redemption_evaluation_fk FOREIGN KEY(eligibility_evaluation_id,family_id,child_id) REFERENCES reward_period_evaluations(id,family_id,child_id) ON DELETE RESTRICT;
CREATE INDEX redemptions_evaluation_count ON reward_redemptions(eligibility_evaluation_id);

CREATE OR REPLACE FUNCTION prevent_reward_evaluation_rewrite() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'reward eligibility evaluations are immutable';
END $$;
CREATE TRIGGER reward_evaluations_immutable BEFORE UPDATE OR DELETE ON reward_period_evaluations FOR EACH ROW EXECUTE FUNCTION prevent_reward_evaluation_rewrite();
CREATE TRIGGER reward_rule_results_immutable BEFORE UPDATE OR DELETE ON reward_evaluation_rule_results FOR EACH ROW EXECUTE FUNCTION prevent_reward_evaluation_rewrite();
CREATE TRIGGER reward_adjustments_immutable BEFORE UPDATE OR DELETE ON reward_evaluation_adjustments FOR EACH ROW EXECUTE FUNCTION prevent_reward_evaluation_rewrite();

CREATE OR REPLACE FUNCTION record_reward_evaluation_reversal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.kind='approval_reversal' THEN
    INSERT INTO reward_evaluation_adjustments(evaluation_id,ledger_entry_id,points_delta,approved_count_delta,reason)
    SELECT e.id,NEW.id,NEW.amount::integer,-1,coalesce(NEW.reason,'Approval reversed')
    FROM reward_period_evaluations e
    JOIN occurrences o ON o.id=NEW.occurrence_id
    JOIN point_ledger award ON award.id=NEW.reverses_entry_id AND award.kind='award'
    WHERE e.family_id=NEW.family_id AND e.child_id=NEW.child_id
      AND o.local_date BETWEEN e.collection_start AND e.collection_end
      AND award.created_at<=e.evaluation_cutoff
    ON CONFLICT(ledger_entry_id) DO NOTHING;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER reward_evaluation_reversal_adjustment AFTER INSERT ON point_ledger FOR EACH ROW EXECUTE FUNCTION record_reward_evaluation_reversal();
