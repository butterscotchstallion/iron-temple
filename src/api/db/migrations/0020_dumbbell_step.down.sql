BEGIN;

-- Dropping the column loses every rack that is not the assumed one, and that
-- loss is real: a lifter who told the app their bells step 2.5 goes back to
-- being prescribed jumps of 10, silently. Nothing else can be done — the fact
-- has no other home, and 0013's tables have no spare column to park it in.
--
-- What does not break is the engine. progression.GymSteps treats a zero or
-- absent step as "not configured" and falls back to DumbbellIncrementLb, which
-- is the number that was hardcoded before this migration existed, so a rolled
-- back database prescribes exactly what it did before it was rolled forward.
ALTER TABLE user_gym DROP COLUMN dumbbell_step_lb;

COMMIT;
