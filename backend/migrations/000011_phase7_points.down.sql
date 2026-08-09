DROP INDEX IF EXISTS completion_attempts_report;
DROP INDEX IF EXISTS occurrences_child_date_history;
DROP INDEX IF EXISTS completion_attempts_pending_queue;
DROP TRIGGER IF EXISTS point_ledger_append_only ON point_ledger;
DROP FUNCTION IF EXISTS prevent_point_ledger_mutation();
ALTER TABLE completion_attempts DROP CONSTRAINT attempts_decision_shape;
ALTER TABLE completion_attempts ADD CONSTRAINT attempts_decision_shape CHECK (
  (decision = 'pending' AND decided_at IS NULL AND decided_by IS NULL) OR
  (decision = 'withdrawn' AND decided_at IS NOT NULL AND decided_by IS NULL) OR
  (decision IN ('approved', 'rejected') AND decided_at IS NOT NULL AND decided_by IS NOT NULL) OR
  (decision = 'cancelled' AND decided_at IS NOT NULL)
);
