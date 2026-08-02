BEGIN;

-- Exercises used across the StrongLifts programs.
INSERT INTO exercises (name) VALUES
    ('Squat'),
    ('Bench Press'),
    ('Barbell Row'),
    ('Overhead Press'),
    ('Deadlift')
ON CONFLICT (name) DO NOTHING;

-- Programs (all linear progression).
INSERT INTO programs (name, description) VALUES
    ('StrongLifts 5x5',      'Three lifts per workout, five sets of five, alternating A/B days.'),
    ('StrongLifts 5x5 Lite', 'Reduced-volume 5x5: two sets of five per lift.'),
    ('Advanced 3x5',         'Graduation fork when 5x5 stalls: three sets of five.')
ON CONFLICT (name) DO NOTHING;

-- Days per program.
INSERT INTO program_days (program_id, name, position)
SELECT p.id, d.name, d.position
FROM (VALUES
    ('StrongLifts 5x5',      'Workout A', 1),
    ('StrongLifts 5x5',      'Workout B', 2),
    ('StrongLifts 5x5 Lite', 'Workout A', 1),
    ('StrongLifts 5x5 Lite', 'Workout B', 2),
    ('Advanced 3x5',         'Workout A', 1),
    ('Advanced 3x5',         'Workout B', 2)
) AS d(program, name, position)
JOIN programs p ON p.name = d.program
ON CONFLICT (program_id, name) DO NOTHING;

-- Prescribed exercises per day:
--   (program, day, exercise, position, sets, reps, starting_weight_lb)
-- Starting weights are conventional empty-bar / light defaults in pounds.
INSERT INTO program_day_exercises
    (program_day_id, exercise_id, position, sets, reps, starting_weight_lb)
SELECT pd.id, e.id, v.position, v.sets, v.reps, v.starting_weight_lb
FROM (VALUES
    -- StrongLifts 5x5
    ('StrongLifts 5x5','Workout A','Squat',          1, 5, 5, 45.0),
    ('StrongLifts 5x5','Workout A','Bench Press',    2, 5, 5, 45.0),
    ('StrongLifts 5x5','Workout A','Barbell Row',    3, 5, 5, 65.0),
    ('StrongLifts 5x5','Workout B','Squat',          1, 5, 5, 45.0),
    ('StrongLifts 5x5','Workout B','Overhead Press', 2, 5, 5, 45.0),
    ('StrongLifts 5x5','Workout B','Deadlift',       3, 1, 5, 95.0),
    -- StrongLifts 5x5 Lite
    ('StrongLifts 5x5 Lite','Workout A','Squat',          1, 2, 5, 45.0),
    ('StrongLifts 5x5 Lite','Workout A','Bench Press',    2, 2, 5, 45.0),
    ('StrongLifts 5x5 Lite','Workout A','Barbell Row',    3, 2, 5, 65.0),
    ('StrongLifts 5x5 Lite','Workout B','Squat',          1, 2, 5, 45.0),
    ('StrongLifts 5x5 Lite','Workout B','Overhead Press', 2, 2, 5, 45.0),
    ('StrongLifts 5x5 Lite','Workout B','Deadlift',       3, 2, 5, 95.0),
    -- Advanced 3x5
    ('Advanced 3x5','Workout A','Squat',          1, 3, 5, 45.0),
    ('Advanced 3x5','Workout A','Bench Press',    2, 3, 5, 45.0),
    ('Advanced 3x5','Workout A','Barbell Row',    3, 3, 5, 65.0),
    ('Advanced 3x5','Workout B','Squat',          1, 3, 5, 45.0),
    ('Advanced 3x5','Workout B','Overhead Press', 2, 3, 5, 45.0),
    ('Advanced 3x5','Workout B','Deadlift',       3, 1, 5, 95.0)
) AS v(program, day, exercise, position, sets, reps, starting_weight_lb)
JOIN programs p      ON p.name = v.program
JOIN program_days pd ON pd.program_id = p.id AND pd.name = v.day
JOIN exercises e     ON e.name = v.exercise
ON CONFLICT (program_day_id, exercise_id) DO NOTHING;

COMMIT;
