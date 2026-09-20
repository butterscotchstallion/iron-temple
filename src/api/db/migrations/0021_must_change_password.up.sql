BEGIN;

-- Whether this account must set a new password before it can use the app.
--
-- 0005 made registration first-user-only, which left no way to add a second
-- account at all. The admin area adds one, and it hands the admin a choice it
-- should not have to make well: they type the new lifter's first password, so
-- for a window that password is known to two people and was chosen by the one
-- who will never use it. Flagging the account closes the window — the password
-- the admin picked is a one-time credential, good for exactly the sign-in that
-- replaces it.
--
-- The flag is what the API gates on, not a hint for the UI. An account carrying
-- it can reach /me (to learn that it must change) and PUT /me/password (to stop
-- carrying it); everything else answers 403. UpdateUserPassword clears it, which
-- is why the opportunistic re-hash on login uses a different query — that one
-- runs on a successful login with the OLD password, and clearing the flag there
-- would let the one-time credential become permanent by being used.
--
-- Defaults false, so every account that already exists is unaffected and the
-- ordinary self-registered owner never sees the screen.
--
-- Added at the end of the table on purpose: db/queries/users.sql lists columns
-- in table order so sqlc returns the User model rather than a bespoke row
-- struct, and that only holds while new columns are appended.
ALTER TABLE users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT false;

COMMIT;
