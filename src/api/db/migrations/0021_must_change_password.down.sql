BEGIN;

-- Dropping the column downgrades every pending forced change to "no change
-- required", and that loss is real: an account the admin created an hour ago,
-- still carrying the password the admin chose for it, becomes an ordinary
-- account with a password two people know and no prompt to fix it. There is
-- nowhere else to park the fact — it is a property of the account, and 0005's
-- tables have no spare column for it.
--
-- What does not break is access. The gate reads this column and nothing else,
-- so a rolled back database simply has no gate: every account reaches every
-- endpoint exactly as it did before this migration existed. Accounts created by
-- the admin area keep working, which is the right failure — the alternative
-- would be locking them out of an app that can no longer tell them why.
ALTER TABLE users DROP COLUMN must_change_password;

COMMIT;
