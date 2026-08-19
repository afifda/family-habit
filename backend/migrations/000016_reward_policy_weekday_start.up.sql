ALTER TABLE reward_eligibility_policies
  DROP CONSTRAINT IF EXISTS reward_policy_week_start_valid,
  ADD CONSTRAINT reward_policy_week_start_valid CHECK(week_starts_on BETWEEN 0 AND 6);
