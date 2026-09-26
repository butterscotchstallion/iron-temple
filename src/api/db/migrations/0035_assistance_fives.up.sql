BEGIN;

-- Take existing assistance work off the rep range and put it on fives.
--
-- 0014 added the range as an opt-in that nothing defaulted to. It later became
-- the picker's default, on an argument about dumbbells: a pair steps 10 lb, the
-- step cannot be made finer, so the only gentle lever left is to advance less
-- OFTEN — which is what climbing 8 to 12 inside one weight does.
--
-- The argument was sound and its premise was wrong. The pair never stepped 10 in
-- the lifter's hand; it stepped one bell, 30 to 35, and the app was reporting one
-- jump as two by showing the whole load. The client now reads a dumbbell per
-- bell, which leaves the range solving a problem that was a display bug.
--
-- What the default cost in the meantime was legibility. A ranged lift prescribes
-- the BOTTOM of its range, so every accessory read "8 reps" — beside programs
-- whose every seeded lift is 5 (see 0002) — with nothing on the workout screen
-- naming a range or asking anyone to climb to 12. Not a gentler pace: an
-- unexplained number. Lifters read it as arbitrary because it was.
--
-- So the rows the default created go back to fives, matching the lifts they sit
-- under. The range itself stays — columns, constraint and engine are untouched —
-- as the opt-in it was built as, for a lift somebody deliberately wants to climb.
UPDATE program_day_assistance
SET reps = 5,
    rep_min = NULL,
    rep_max = NULL
WHERE rep_min IS NOT NULL;

-- Only ranged rows, and this is the whole point of the WHERE clause.
--
-- An unranged row's `reps` was typed by a lifter and means something: band
-- pull-aparts at 15, a plank at 30. Those never showed the confusing 8 — that
-- number came from the range's bottom — so they have nothing to be rescued from,
-- and rewriting them to 5 would be this migration inventing a prescription
-- nobody asked for. A ranged row's 8, by contrast, is the picker's default
-- looking back at us.
--
-- Nulling both columns together satisfies program_day_assistance_rep_range_ck,
-- which requires exactly that.

COMMIT;
