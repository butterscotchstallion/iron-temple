BEGIN;

-- What the lifter weighed on the day of the session — the other half of a
-- training log, and the series a weight-loss chart reads.
--
-- NULL means "no weigh-in", and that is the point rather than an oversight. The
-- session screen pre-fills the box with the last recorded value so a new session
-- needs a nudge and not a fresh entry, but it writes nothing until the lifter
-- actually edits it. Carrying the number forward into the row itself would be
-- easier and wrong: every session would then hold a weight, and a chart of
-- "bodyweight over time" would draw a flat line through the days nobody stood on
-- a scale, indistinguishable from the days they did.
--
-- NUMERIC(6,2) to match every other weight in the schema (session_sets.weight_lb,
-- program_day_exercises.starting_weight_lb), even though a person is nowhere near
-- the four digits that allows. The CHECK rejects zero as well as negatives: unlike
-- assistance weight, where 0 legitimately means bodyweight work, a lifter who
-- weighs nothing has mistyped.
ALTER TABLE sessions
    ADD COLUMN bodyweight_lb NUMERIC(6,2) CHECK (bodyweight_lb > 0);

-- Serves exactly one query, LastWeighIn: "this lifter's most recent weigh-in".
-- Partial, so it indexes only the sessions that can answer it — on a log where
-- the scale is a sometimes thing, that is a small fraction of the rows, and the
-- sessions with no weigh-in are precisely the ones the query skips. The column
-- order matches the query's ORDER BY (performed_on DESC, id DESC) so the first
-- row read is the answer.
CREATE INDEX sessions_bodyweight_idx
    ON sessions (user_id, performed_on DESC, id DESC)
    WHERE bodyweight_lb IS NOT NULL;

COMMIT;
