UPDATE children SET avatar='fox' WHERE avatar IS NULL;
UPDATE children SET color='#4F46E5' WHERE color IS NULL;

ALTER TABLE children
  ALTER COLUMN avatar SET NOT NULL,
  ALTER COLUMN color SET NOT NULL,
  ADD CONSTRAINT children_nickname_length CHECK (char_length(btrim(nickname)) BETWEEN 1 AND 40),
  ADD CONSTRAINT children_nickname_trimmed CHECK (nickname = btrim(nickname)),
  ADD CONSTRAINT children_avatar_valid CHECK (avatar IN ('fox','bear','rabbit','owl','cat','elephant','panda','koala')),
  ADD CONSTRAINT children_color_valid CHECK (color ~ '^#[0-9A-Fa-f]{6}$'),
  ADD CONSTRAINT children_pin_hash_not_blank CHECK (pin_hash IS NULL OR btrim(pin_hash) <> '');

CREATE INDEX children_family_active_created
  ON children (family_id, created_at, id) WHERE archived_at IS NULL;

DROP INDEX children_active_nickname_unique;
CREATE UNIQUE INDEX children_active_nickname_unique
  ON children (family_id, lower(btrim(nickname))) WHERE archived_at IS NULL;
