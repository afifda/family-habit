DROP TRIGGER IF EXISTS point_ledger_link_guard ON point_ledger;
DROP FUNCTION IF EXISTS validate_ledger_links();
CREATE TRIGGER point_ledger_reversal_guard
  BEFORE INSERT ON point_ledger FOR EACH ROW EXECUTE FUNCTION validate_ledger_reversal();
DROP INDEX IF EXISTS point_ledger_one_award_attempt;
