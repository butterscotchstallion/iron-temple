BEGIN;

-- Dropping the three steps loses every stack a lifter has described, the same
-- way 0020's down migration loses their dumbbell rack, and the loss is real in
-- the same way: a lifter who said their machine steps 15 goes back to being
-- prescribed the bar's jumps without being told. The fact has no other home.
--
-- What does not break is the engine. progression.GymSteps reads a zero or
-- absent value as "not configured" and falls back to the per-kind constants,
-- which for machine, cable and band are all 5 — and 5 is what BarLb already
-- resolves to for a standard plate set. So a rolled back database prescribes
-- what it did before it was rolled forward, for everyone except the lifter with
-- unusually fine plates, who goes back to the wrong answer this migration was
-- written to fix.
--
-- equipment_confirmed_at is different, and worth naming separately: rolling back
-- does not just forget the timestamps, it forgets the DISTINCTION. Every gym on
-- the install becomes indistinguishable from a guess again, which is the state
-- that let a seeded 35 lb plate pass for a fact. Rolling forward a second time
-- starts everyone at NULL, so a lifter who already confirmed their equipment is
-- asked to confirm it once more. That is one screen, and it is the only honest
-- answer available — the column is the only place that knowledge ever lived.

ALTER TABLE user_gym
    DROP COLUMN machine_step_lb,
    DROP COLUMN cable_step_lb,
    DROP COLUMN band_step_lb,
    DROP COLUMN equipment_confirmed_at;

COMMIT;
