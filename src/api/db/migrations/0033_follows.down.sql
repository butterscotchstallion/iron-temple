BEGIN;

-- Drops unconditionally, like 0024 and 0032 and unlike the program
-- down-migrations.
--
-- What is lost is who had chosen to hear about whom, and that is a real loss —
-- nothing else in the database records it, so going down and up again leaves every
-- lifter following nobody. It is accepted for 0024's reason: a follow is something
-- people decided about each other, not the training itself. Every session, set and
-- rep is exactly as it was, and the worst of it is a few button presses.
--
-- Note what this does NOT restore: the crown fan-out. 0033's up migration is only
-- the table; the query that reads it lives in db/queries/achievements.sql, so a
-- deploy rolled back past this also rolls back the code that would look for it.
-- Dropping the table under a server still running the follows-gated query would
-- error every reconcile — which is the ordinary rule for down migrations here and
-- worth stating because this one is a table a hot path reads.
--
-- The index goes with the table; naming it here would be noise.

DROP TABLE IF EXISTS follows;

COMMIT;
