BEGIN;

-- Lossless: an index holds no data of its own. Dropping it makes the reads that
-- start from an exercise — the history endpoint, and the top-set lateral behind
-- the library listing — scan session_sets again, which is where they were
-- before 0016 rather than broken.
DROP INDEX session_sets_exercise_idx;

COMMIT;
