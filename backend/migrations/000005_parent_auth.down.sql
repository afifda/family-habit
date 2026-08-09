DROP INDEX IF EXISTS sessions_token_active;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_csrf_hash_present;
ALTER TABLE sessions DROP COLUMN IF EXISTS csrf_token_hash;
