DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM routine_groups
    WHERE icon IS NOT NULL
      AND icon NOT IN ('🌅','☀️','🏫','🌆','🌙','⭐','🎁','🍦','🎮','🎬','📚','🚲','🍕','🎨','⚽')
  ) THEN
    RAISE EXCEPTION 'cannot restore routine_groups_icon_safe allowlist while custom routine group icons exist';
  END IF;
  IF EXISTS (
    SELECT 1 FROM rewards
    WHERE icon IS NOT NULL
      AND icon NOT IN ('🌅','☀️','🏫','🌆','🌙','⭐','🎁','🍦','🎮','🎬','📚','🚲','🍕','🎨','⚽')
  ) THEN
    RAISE EXCEPTION 'cannot restore rewards_icon_safe allowlist while custom reward icons exist';
  END IF;
END $$;

ALTER TABLE routine_groups DROP CONSTRAINT IF EXISTS routine_groups_icon_safe;
ALTER TABLE routine_groups
  ADD CONSTRAINT routine_groups_icon_safe
  CHECK(icon IS NULL OR icon IN ('🌅','☀️','🏫','🌆','🌙','⭐','🎁','🍦','🎮','🎬','📚','🚲','🍕','🎨','⚽'));

ALTER TABLE rewards DROP CONSTRAINT IF EXISTS rewards_icon_safe;
ALTER TABLE rewards
  ADD CONSTRAINT rewards_icon_safe
  CHECK(icon IS NULL OR icon IN ('🌅','☀️','🏫','🌆','🌙','⭐','🎁','🍦','🎮','🎬','📚','🚲','🍕','🎨','⚽'));
