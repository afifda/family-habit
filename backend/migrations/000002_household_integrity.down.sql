ALTER TABLE audit_events DROP CONSTRAINT IF EXISTS audit_child_family_fk, DROP CONSTRAINT IF EXISTS audit_user_membership_fk;
ALTER TABLE completion_attempts DROP CONSTRAINT IF EXISTS attempts_decider_membership_fk;
ALTER TABLE point_ledger DROP CONSTRAINT IF EXISTS ledger_actor_membership_fk, DROP CONSTRAINT IF EXISTS ledger_child_family_fk;
ALTER TABLE habit_assignments DROP CONSTRAINT IF EXISTS assignments_child_family_fk, DROP CONSTRAINT IF EXISTS assignments_habit_family_fk;
ALTER TABLE habits DROP CONSTRAINT IF EXISTS habits_id_family_unique;
ALTER TABLE children DROP CONSTRAINT IF EXISTS children_id_family_unique;
