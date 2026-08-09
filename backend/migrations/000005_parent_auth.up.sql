ALTER TABLE sessions
  ADD COLUMN csrf_token_hash bytea,
  ADD CONSTRAINT sessions_csrf_hash_present CHECK (csrf_token_hash IS NOT NULL);

CREATE INDEX sessions_token_active ON sessions (token_hash)
  WHERE revoked_at IS NULL;
