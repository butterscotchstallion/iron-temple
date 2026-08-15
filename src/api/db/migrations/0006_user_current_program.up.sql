BEGIN;

-- The program the user last opened, so the app can land them on it instead of
-- asking every time. Before this column the answer was re-derived on the client
-- from the most recent session, which meant opening a program without logging a
-- set left no trace, and two components computed it independently.
--
-- Nullable for the same reason sessions.user_id is (see 0005_users.up.sql): no
-- existing row has an answer, and NOT NULL would mean picking a program on the
-- user's behalf. NULL reads as "not chosen yet", which is what the UI falls back
-- from — it still derives the program from session history in that case, so the
-- home screen of an existing account does not change until they next open one.
--
-- ON DELETE SET NULL rather than CASCADE or RESTRICT: a removed program should
-- cost the user a preference, not their account, and it should not be able to
-- block the delete either. They fall back to the derived value.
ALTER TABLE users
    ADD COLUMN current_program_id INTEGER REFERENCES programs(id) ON DELETE SET NULL;

COMMIT;
