DROP INDEX IF EXISTS children_family_active_created;
DROP INDEX IF EXISTS children_active_nickname_unique;
CREATE UNIQUE INDEX children_active_nickname_unique
  ON children (family_id, lower(nickname)) WHERE archived_at IS NULL;
ALTER TABLE children
  ALTER COLUMN avatar DROP NOT NULL,
  ALTER COLUMN color DROP NOT NULL,
  DROP CONSTRAINT IF EXISTS children_color_valid,
  DROP CONSTRAINT IF EXISTS children_pin_hash_not_blank,
  DROP CONSTRAINT IF EXISTS children_avatar_valid,
  DROP CONSTRAINT IF EXISTS children_nickname_trimmed,
  DROP CONSTRAINT IF EXISTS children_nickname_length;
