DROP TABLE IF EXISTS audit_events, point_ledger, completion_attempts, occurrences, habit_schedules,
  habit_assignments, habits, children, family_memberships, families, users CASCADE;
DROP TYPE IF EXISTS ledger_kind, completion_decision, occurrence_state, schedule_kind, membership_role;
