BEGIN;

-- Look sets up by the LIFT they were performed on, not just by the session they
-- belong to.
--
-- 0001 indexed session_sets (session_id), which serves "the sets in this
-- session" — the read the app made most. Every read that starts from an
-- exercise instead had no index and fell back to scanning the table:
-- ListExerciseHistory does it once per call, and ListExercises' top-set lateral
-- does it ONCE PER ROW, so listing the library ran a full scan of every set the
-- lifter has ever logged 53 times over.
--
-- (exercise_id, session_id) rather than (exercise_id) alone: both callers filter
-- on the exercise and then immediately join sessions on session_id, so carrying
-- it in the index feeds the join from the same read rather than sending the
-- planner back to the heap for it.
--
-- Not partial on actual_reps > 0, though both queries carry that predicate. The
-- column is NULL until a set is logged and most rows in a live session are
-- unlogged, so it is tempting — but a partial index would stop serving any
-- future read that wants prescribed sets too, to save a fraction of a small
-- table. The plain index is the one that cannot become wrong.
--
-- Measured on a seeded copy (300 sessions, 7,500 sets, the 53-exercise seeded
-- catalogue), listing the library with the top-set lateral:
--
--   before   33.7 ms, 2,985 shared buffers, Seq Scan on session_sets x53
--   after     5.6 ms,   413 shared buffers, Bitmap Index Scan x53
CREATE INDEX session_sets_exercise_idx ON session_sets (exercise_id, session_id);

COMMIT;
