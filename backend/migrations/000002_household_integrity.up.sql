ALTER TABLE children ADD CONSTRAINT children_id_family_unique UNIQUE (id, family_id);
ALTER TABLE habits ADD CONSTRAINT habits_id_family_unique UNIQUE (id, family_id);

ALTER TABLE habit_assignments
  ADD CONSTRAINT assignments_habit_family_fk FOREIGN KEY (habit_id, family_id)
    REFERENCES habits(id, family_id) ON DELETE RESTRICT,
  ADD CONSTRAINT assignments_child_family_fk FOREIGN KEY (child_id, family_id)
    REFERENCES children(id, family_id) ON DELETE RESTRICT;

ALTER TABLE point_ledger
  ADD CONSTRAINT ledger_child_family_fk FOREIGN KEY (child_id, family_id)
    REFERENCES children(id, family_id) ON DELETE RESTRICT,
  ADD CONSTRAINT ledger_actor_membership_fk FOREIGN KEY (family_id, actor_user_id)
    REFERENCES family_memberships(family_id, user_id) ON DELETE RESTRICT;

ALTER TABLE completion_attempts
  ADD CONSTRAINT attempts_decider_membership_fk FOREIGN KEY (family_id, decided_by)
    REFERENCES family_memberships(family_id, user_id) ON DELETE RESTRICT;

ALTER TABLE audit_events
  ADD CONSTRAINT audit_user_membership_fk FOREIGN KEY (family_id, actor_user_id)
    REFERENCES family_memberships(family_id, user_id) ON DELETE RESTRICT,
  ADD CONSTRAINT audit_child_family_fk FOREIGN KEY (actor_child_id, family_id)
    REFERENCES children(id, family_id) ON DELETE RESTRICT;
