CREATE OR REPLACE FUNCTION validate_ledger_reversal() RETURNS trigger LANGUAGE plpgsql AS $$
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

ALTER TABLE point_ledger RENAME COLUMN amount TO points;
