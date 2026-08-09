CREATE TABLE authentication_attempts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  identifier_hash bytea NOT NULL,
  source_hash bytea NOT NULL,
  outcome text NOT NULL CHECK (outcome IN ('failed', 'rate_limited')),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX authentication_attempts_created_at ON authentication_attempts (created_at);
