BEGIN;

-- One definition of an estimated one-rep max, in the database.
--
-- The Epley estimate had been written out by hand in three queries —
-- RackedExerciseBaseline, RecapExerciseBaseline and ListSessionPersonalBests —
-- each carrying the same ROUND, the same single-rep CASE, and a comment on top
-- saying the copies must stay identical. They must: Set.E1RM in Go rounds to the
-- pound and compares straight against these numbers, so a change on one side and
-- not the others makes the two disagree inside a sub-pound band, and that band is
-- exactly where a record is decided. Unrounded on one side, repeating an
-- identical set reads as a new record — 185 x 3 is 203.5, Go calls it 204, and
-- 204 > 203.5.
--
-- An invariant maintained by three comments asking people to be careful is one
-- that breaks the first time somebody edits two of the three. This makes it
-- structural: there is now one body, and a query that wants the estimate calls
-- it. The Go (racked.Set.E1RM) and TypeScript (oneRepMax.ts) copies necessarily
-- remain — they run where there is no database, one for the reporter and one for
-- the offline recap — so this removes two of the four copies, not all of them.
--
-- IMMUTABLE is true and load-bearing: the result depends on nothing but the two
-- arguments, which is what lets the planner fold it into an index condition and
-- what makes it legal in an index expression should one ever be wanted. PARALLEL
-- SAFE for the same reason — these queries aggregate over every set a lifter has
-- ever logged, and that is the shape of scan a parallel plan is for.
--
-- STRICT (RETURNS NULL ON NULL INPUT) matches the column: actual_reps is NULL
-- until a set is logged. Every caller already filters those out with
-- `actual_reps > 0`, so this is a backstop rather than the behaviour anything
-- relies on — but without it the CASE would return NULL anyway, and being
-- explicit lets the planner skip the call entirely rather than evaluate it.
--
-- The single is spelled out rather than left to Epley for the reason the CASE has
-- always existed: Epley extrapolates from a set carried past one rep, and at
-- exactly one rep it inflates a known number by a thirtieth. A 225 single is a
-- 225 estimated max, not a 233 one — and it should not outrank a 225 double,
-- which is plainly the harder set.
--
-- Rounding per row rather than around the MAX mirrors what Go does, and is
-- equivalent anyway since ROUND is monotonic.
CREATE FUNCTION e1rm_lb(weight_lb NUMERIC, reps INTEGER)
RETURNS NUMERIC
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
RETURNS NULL ON NULL INPUT
AS $$
    SELECT ROUND(CASE WHEN reps = 1
                      THEN weight_lb
                      ELSE weight_lb * (1 + reps / 30.0)
                 END)
$$;

COMMENT ON FUNCTION e1rm_lb(NUMERIC, INTEGER) IS
    'Epley estimated one-rep max, rounded to the pound, with a single worth what '
    'was on the bar. Mirrored by racked.Set.E1RM (Go) and oneRepMax.ts (UI); a '
    'change here is a change there.';

COMMIT;
