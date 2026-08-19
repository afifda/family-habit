ALTER TABLE routine_groups DROP CONSTRAINT IF EXISTS routine_groups_icon_safe;
ALTER TABLE routine_groups
  ADD CONSTRAINT routine_groups_icon_safe
  CHECK(icon IS NULL OR (char_length(icon) <= 40 AND icon !~ '[[:cntrl:]<>]'));

ALTER TABLE rewards DROP CONSTRAINT IF EXISTS rewards_icon_safe;
ALTER TABLE rewards
  ADD CONSTRAINT rewards_icon_safe
  CHECK(icon IS NULL OR (char_length(icon) <= 40 AND icon !~ '[[:cntrl:]<>]'));
