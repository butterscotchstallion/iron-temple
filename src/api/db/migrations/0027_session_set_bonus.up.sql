BEGIN;

-- Whether this set was bonus work: an extra set added during the session, after
-- everything else in it was already done.
--
-- The distinction cannot be derived, which is the whole reason for a column.
-- session_sets carries no timestamp and no record of where a row came from, so
-- the prescription written by CreateSessionSet when the session opens and the
-- extra set written by AppendSessionSet an hour later are, afterwards,
-- identical rows. `completed` has no history either — it is a boolean that ends
-- up true, with nothing to say when it turned. So "was the rest of the workout
-- finished when this set was added?" is a question only the moment of the
-- append can answer, and this column is where that answer is kept.
--
-- The consequence is worth stating: every set logged before this migration
-- reads false, and no backfill can do better than guess. Set number 4 of a
-- 3-set prescription was certainly ADDED, but whether it was added as bonus
-- work at the end or as a mid-workout adjustment is exactly the part that was
-- never recorded. A recap of an old session therefore reports no bonus sets
-- rather than a plausible invention.
--
-- NOT NULL DEFAULT false rather than a nullable "unknown", because the two
-- readings a NULL would carry — "not bonus" and "from before we tracked it" —
-- are not distinguished by anything downstream. Every caller wants a count, and
-- a count of unknowns is not a number a recap can show. The default is also
-- what keeps the seeding path honest without touching it: a set nobody marked
-- is not a bonus set, which is precisely true of every set a session opens with.
ALTER TABLE session_sets
    ADD COLUMN is_bonus BOOLEAN NOT NULL DEFAULT false;

COMMIT;
