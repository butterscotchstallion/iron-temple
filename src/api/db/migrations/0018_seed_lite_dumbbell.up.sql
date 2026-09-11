BEGIN;

-- StrongLifts 5x5 Lite, pressed with dumbbells.
--
-- A clone of the Lite program 0002 seeded, identical in every respect but one:
-- Workout B's Overhead Press becomes a dumbbell press. Everything else — two
-- days, two sets of five, the same lifts in the same order at the same starting
-- weights — is copied deliberately rather than shared, because programs are
-- immutable seed data (see 0009's design note) and there is no mechanism for one
-- program to inherit another's days. A clone is the only shape available, and
-- the duplication is the price of the prescriptions staying canonical.

-- ---------------------------------------------------------------------------
-- 1. Promote the lift.
-- ---------------------------------------------------------------------------
--
-- 'Dumbbell Shoulder Press' already exists: 0009 seeded it into the accessory
-- catalogue as shoulders/dumbbell, and 0011 gave it a compound's 180s rest
-- rather than an isolation movement's 90. It is reused rather than duplicated
-- under the name 'Dumbbell Overhead Press' — the catalogue holds one row per
-- movement (0009 made name uniqueness case-insensitive precisely so that it
-- would), and history follows the lift, so a lifter who has been pressing
-- dumbbells as assistance keeps that history when a program starts prescribing
-- them instead of starting from an empty one.
--
-- is_accessory is 0009's distinction between what a program puts on the bar and
-- what a lifter bolts on afterwards. A program now prescribes this, so it is no
-- longer an accessory. rest_seconds is untouched and stays at the 180 that 0011
-- gave it, which is already what every other prescribed lift rests after 0017's
-- cap — the promotion costs it nothing.
UPDATE exercises
   SET is_accessory = false
 WHERE name = 'Dumbbell Shoulder Press'
   AND created_by_user_id IS NULL;

-- ---------------------------------------------------------------------------
-- 2. The program.
-- ---------------------------------------------------------------------------
INSERT INTO programs (name, description) VALUES
    ('StrongLifts 5x5 Lite (Dumbbell Press)',
     'Reduced-volume 5x5 with the overhead press taken to dumbbells: two sets of five per lift.')
ON CONFLICT (name) DO NOTHING;

INSERT INTO program_days (program_id, name, position)
SELECT p.id, d.name, d.position
FROM (VALUES
    ('StrongLifts 5x5 Lite (Dumbbell Press)', 'Workout A', 1),
    ('StrongLifts 5x5 Lite (Dumbbell Press)', 'Workout B', 2)
) AS d(program, name, position)
JOIN programs p ON p.name = d.program
ON CONFLICT (program_id, name) DO NOTHING;

-- Same shape as 0002 and 0007: (program, day, exercise, position, sets, reps,
-- starting_weight_lb), and the same empty-bar / light starting weights for the
-- four barbell lifts.
--
-- The dumbbell press starts at 30 lb, read as the PAIR — both bells together,
-- the same way every other weight in this app is the whole load rather than one
-- side of it. That keeps Racked's tonnage comparable across lifts, which it
-- would not be if this one number alone meant per-hand. A lifter whose bells
-- start somewhere else overrides it with a baseline (/me/baselines), which is
-- the same escape hatch the 45 lb bar assumption has.
INSERT INTO program_day_exercises
    (program_day_id, exercise_id, position, sets, reps, starting_weight_lb)
SELECT pd.id, e.id, v.position, v.sets, v.reps, v.starting_weight_lb
FROM (VALUES
    ('StrongLifts 5x5 Lite (Dumbbell Press)','Workout A','Squat',                  1, 2, 5, 45.0),
    ('StrongLifts 5x5 Lite (Dumbbell Press)','Workout A','Bench Press',            2, 2, 5, 45.0),
    ('StrongLifts 5x5 Lite (Dumbbell Press)','Workout A','Barbell Row',            3, 2, 5, 65.0),
    ('StrongLifts 5x5 Lite (Dumbbell Press)','Workout B','Squat',                  1, 2, 5, 45.0),
    ('StrongLifts 5x5 Lite (Dumbbell Press)','Workout B','Dumbbell Shoulder Press',2, 2, 5, 30.0),
    ('StrongLifts 5x5 Lite (Dumbbell Press)','Workout B','Deadlift',               3, 2, 5, 95.0)
) AS v(program, day, exercise, position, sets, reps, starting_weight_lb)
JOIN programs p      ON p.name = v.program
JOIN program_days pd ON pd.program_id = p.id AND pd.name = v.day
-- Matched against the shared catalogue only. Without the created_by_user_id
-- filter a lifter's own 'Dumbbell Shoulder Press' — which 0009's per-owner
-- uniqueness permits — could join here too and seed the day twice.
JOIN exercises e     ON e.name = v.exercise AND e.created_by_user_id IS NULL
ON CONFLICT (program_day_id, exercise_id) DO NOTHING;

COMMIT;
