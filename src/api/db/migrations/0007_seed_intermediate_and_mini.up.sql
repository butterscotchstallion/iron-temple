BEGIN;

-- Variation lifts introduced by the Intermediate program.
INSERT INTO exercises (name) VALUES
    ('Pause Squat'),
    ('Pause Bench Press'),
    ('Pause Deadlift'),
    ('Incline Bench Press'),
    ('Feet-Up Bench Press')
ON CONFLICT (name) DO NOTHING;

INSERT INTO programs (name, description) VALUES
    ('StrongLifts 5x5 Intermediate',
     'Three-day A/B/C split: heavy, volume and paused work. Assistance work is the lifter''s choice.'),
    ('StrongLifts 5x5 Mini',
     'Two lifts per workout, two sets of five, alternating A/B days.')
ON CONFLICT (name) DO NOTHING;

INSERT INTO program_days (program_id, name, position)
SELECT p.id, d.name, d.position
FROM (VALUES
    ('StrongLifts 5x5 Intermediate', 'Workout A', 1),
    ('StrongLifts 5x5 Intermediate', 'Workout B', 2),
    ('StrongLifts 5x5 Intermediate', 'Workout C', 3),
    ('StrongLifts 5x5 Mini',         'Workout A', 1),
    ('StrongLifts 5x5 Mini',         'Workout B', 2)
) AS d(program, name, position)
JOIN programs p ON p.name = d.program
ON CONFLICT (program_id, name) DO NOTHING;

-- Same shape as 0002: (program, day, exercise, position, sets, reps, starting_weight_lb),
-- with the same empty-bar / light starting weights. The Intermediate program also
-- prescribes assistance work on every day; that is deliberately unseeded — it has no
-- fixed lift, sets or reps, and the schema has nowhere to put a prescription without them.
INSERT INTO program_day_exercises
    (program_day_id, exercise_id, position, sets, reps, starting_weight_lb)
SELECT pd.id, e.id, v.position, v.sets, v.reps, v.starting_weight_lb
FROM (VALUES
    -- StrongLifts 5x5 Intermediate
    ('StrongLifts 5x5 Intermediate','Workout A','Squat',               1, 5, 5, 45.0),
    ('StrongLifts 5x5 Intermediate','Workout A','Bench Press',         2, 5, 5, 45.0),
    ('StrongLifts 5x5 Intermediate','Workout A','Barbell Row',         3, 5, 8, 65.0),
    ('StrongLifts 5x5 Intermediate','Workout B','Deadlift',            1, 5, 5, 95.0),
    ('StrongLifts 5x5 Intermediate','Workout B','Incline Bench Press', 2, 5, 8, 45.0),
    ('StrongLifts 5x5 Intermediate','Workout B','Feet-Up Bench Press', 3, 5, 8, 45.0),
    ('StrongLifts 5x5 Intermediate','Workout C','Pause Squat',         1, 5, 3, 45.0),
    ('StrongLifts 5x5 Intermediate','Workout C','Pause Bench Press',   2, 5, 3, 45.0),
    ('StrongLifts 5x5 Intermediate','Workout C','Pause Deadlift',      3, 2, 3, 95.0),
    -- StrongLifts 5x5 Mini
    ('StrongLifts 5x5 Mini','Workout A','Squat',          1, 2, 5, 45.0),
    ('StrongLifts 5x5 Mini','Workout A','Bench Press',    2, 2, 5, 45.0),
    ('StrongLifts 5x5 Mini','Workout B','Deadlift',       1, 2, 5, 95.0),
    ('StrongLifts 5x5 Mini','Workout B','Overhead Press', 2, 2, 5, 45.0)
) AS v(program, day, exercise, position, sets, reps, starting_weight_lb)
JOIN programs p      ON p.name = v.program
JOIN program_days pd ON pd.program_id = p.id AND pd.name = v.day
JOIN exercises e     ON e.name = v.exercise
ON CONFLICT (program_day_id, exercise_id) DO NOTHING;

COMMIT;
