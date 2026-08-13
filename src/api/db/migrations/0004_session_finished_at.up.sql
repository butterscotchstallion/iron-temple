BEGIN;

-- When the lifter explicitly ended the session (the Finish button). NULL means
-- it was never finished by hand; such a session is still considered over once
-- it is more than 12 hours old — see the is_over expression in
-- db/queries/sessions.sql. Deliberately named apart from session_sets.completed,
-- which means "this set hit its target reps".
ALTER TABLE sessions
    ADD COLUMN finished_at TIMESTAMPTZ;

COMMIT;
