ALTER TABLE completion_attempts DROP CONSTRAINT attempts_decision_shape;
ALTER TABLE completion_attempts ADD CONSTRAINT attempts_decision_shape CHECK (
  (decision = 'pending' AND decided_at IS NULL AND decided_by IS NULL) OR
  (decision = 'withdrawn' AND decided_at IS NOT NULL AND decided_by IS NULL) OR
  (decision IN ('approved', 'rejected') AND decided_at IS NOT NULL AND decided_by IS NOT NULL) OR
  (decision = 'cancelled' AND decided_at IS NOT NULL)
);

CREATE OR REPLACE FUNCTION prevent_point_ledger_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'point ledger is append-only';
END $$;
CREATE TRIGGER point_ledger_append_only
  BEFORE UPDATE OR DELETE ON point_ledger FOR EACH ROW EXECUTE FUNCTION prevent_point_ledger_mutation();

CREATE INDEX completion_attempts_pending_queue
  ON completion_attempts (family_id, submitted_at, id) WHERE decision = 'pending';
CREATE INDEX occurrences_child_date_history
  ON occurrences (family_id, child_id, local_date DESC, id DESC);
CREATE INDEX completion_attempts_report
  ON completion_attempts (family_id, child_id, submitted_at, decision);
