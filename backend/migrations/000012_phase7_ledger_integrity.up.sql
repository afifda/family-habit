CREATE UNIQUE INDEX point_ledger_one_award_attempt
  ON point_ledger (completion_attempt_id) WHERE kind = 'award';

CREATE OR REPLACE FUNCTION validate_ledger_links() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  original point_ledger%ROWTYPE;
  attempt completion_attempts%ROWTYPE;
  occurrence occurrences%ROWTYPE;
BEGIN
  IF NEW.kind = 'award' THEN
    SELECT * INTO attempt FROM completion_attempts WHERE id = NEW.completion_attempt_id FOR KEY SHARE;
    SELECT * INTO occurrence FROM occurrences WHERE id = NEW.occurrence_id FOR KEY SHARE;
    IF NOT FOUND OR attempt.occurrence_id <> NEW.occurrence_id OR attempt.family_id <> NEW.family_id OR
       attempt.child_id <> NEW.child_id OR attempt.decision <> 'approved' OR occurrence.family_id <> NEW.family_id OR
       occurrence.child_id <> NEW.child_id OR occurrence.points_snapshot <> NEW.amount THEN
      RAISE EXCEPTION 'award must match its approved attempt and occurrence snapshot';
    END IF;
  ELSIF NEW.kind = 'approval_reversal' THEN
    SELECT * INTO original FROM point_ledger WHERE id = NEW.reverses_entry_id FOR KEY SHARE;
    IF NOT FOUND OR original.kind <> 'award' OR original.family_id <> NEW.family_id OR
       original.child_id <> NEW.child_id OR original.occurrence_id <> NEW.occurrence_id OR
       original.amount <> -NEW.amount THEN
      RAISE EXCEPTION 'approval reversal must exactly negate its matching award';
    END IF;
  END IF;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS point_ledger_reversal_guard ON point_ledger;
CREATE TRIGGER point_ledger_link_guard
  BEFORE INSERT ON point_ledger FOR EACH ROW EXECUTE FUNCTION validate_ledger_links();
