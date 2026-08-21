BEGIN;

-- How long to rest after a set of this movement.
--
-- The API has advertised restSeconds on every prescription since the spec was
-- written, but there was no column behind it: dto.go carried a fixed
-- `restSecondsDefault = 180` and every lift got three minutes. That is the right
-- number for a bench press and plainly the wrong one for both a deadlift and a
-- lateral raise, which is what this column fixes.
--
-- It lives on exercises rather than on program_day_exercises because rest is a
-- property of the movement, not of the day it was prescribed on: a squat is as
-- taxing on Workout B as on Workout A, and an accessory is as light whichever
-- day a lifter bolted it onto. That also means assistance work gets a sensible
-- rest for free, without program_day_assistance needing a column of its own.
--
-- The bounds are sanity rails, not a policy: below 30s the timer is noise, and
-- above 15 minutes it is no longer a rest between sets.
ALTER TABLE exercises
    ADD COLUMN rest_seconds INTEGER NOT NULL DEFAULT 180
        CHECK (rest_seconds BETWEEN 30 AND 900);

-- ---------------------------------------------------------------------------
-- Three tiers, applied in order — each UPDATE narrows the one before it.
-- ---------------------------------------------------------------------------
--
-- The column default (180) is tier two and already covers every lift the
-- programs prescribe, so only the two ends need writing. It is also what a
-- lifter's own custom movement inherits, which is the right guess for something
-- we know nothing about beyond its name.

-- Accessory work is lighter and more local than what a program puts on the bar,
-- so it recovers faster. is_accessory is 0009's own distinction between the two,
-- reused rather than re-litigated.
UPDATE exercises SET rest_seconds = 90 WHERE is_accessory;

-- Except the multi-joint accessories. A dip or a leg press is accessory work by
-- 0009's definition — no program prescribes it — but it is still a compound
-- movement under real load, and 90 seconds is not enough of one. Listed by name
-- rather than derived: the catalogue is fixed and seeded a few lines above this
-- in 0009, and a name is easier to audit than a rule inferred from equipment.
UPDATE exercises SET rest_seconds = 180
WHERE name IN (
    'Dumbbell Bench Press', 'Dumbbell Incline Press', 'Machine Chest Press',
    'Push-Up', 'Dip',
    'Pull-Up', 'Chin-Up', 'Lat Pulldown', 'Seated Cable Row', 'Dumbbell Row',
    'T-Bar Row',
    'Leg Press', 'Walking Lunge',
    'Dumbbell Shoulder Press', 'Arnold Press', 'Upright Row',
    'Close-Grip Bench Press'
);

-- The squat and the deadlift families move the most weight and take the longest
-- to recover from — five minutes is the conventional prescription, and the one
-- number a lifter is most likely to have been setting by hand. Goblet and
-- Bulgarian split squats are in the family by name but not by load, so they stay
-- where the tier above left them.
UPDATE exercises SET rest_seconds = 300
WHERE name IN (
    'Squat', 'Pause Squat', 'Front Squat',
    'Deadlift', 'Pause Deadlift', 'Romanian Deadlift'
);

COMMIT;
