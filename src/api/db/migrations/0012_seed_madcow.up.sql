BEGIN;

-- Madcow 5x5: a two-day A/B alternation of fives and triples.
--
-- No new exercises. Every lift it prescribes was already seeded by 0002 under
-- this schema's canonical names, so the (Barbell) qualifiers a lifter sees in
-- other apps collapse onto the existing rows — "Bent Over Row (Barbell)" is
-- this library's 'Barbell Row', and nothing here needs 0009's classification
-- pass or 0011's rest tiers; both already hold for these five lifts.
--
-- That reuse also keeps a squat's history one history: the progression engine
-- reads it by exercise, so a lifter moving here from StrongLifts picks the bar
-- up where they left it rather than restarting at an empty bar.

INSERT INTO programs (name, description) VALUES
    ('Madcow 5x5',
     'Two-day A/B alternation: fives on squat, press and deadlift, then a day of heavy triples.')
ON CONFLICT (name) DO NOTHING;

INSERT INTO program_days (program_id, name, position)
SELECT p.id, d.name, d.position
FROM (VALUES
    ('Madcow 5x5', 'Workout A', 1),
    ('Madcow 5x5', 'Workout B', 2)
) AS d(program, name, position)
JOIN programs p ON p.name = d.program
ON CONFLICT (program_id, name) DO NOTHING;

-- Same shape as 0002 and 0007: (program, day, exercise, position, sets, reps,
-- starting_weight_lb), with the same empty-bar / light starting weights.
INSERT INTO program_day_exercises
    (program_day_id, exercise_id, position, sets, reps, starting_weight_lb)
SELECT pd.id, e.id, v.position, v.sets, v.reps, v.starting_weight_lb
FROM (VALUES
    ('Madcow 5x5','Workout A','Squat',          1, 2, 5, 45.0),
    ('Madcow 5x5','Workout A','Overhead Press', 2, 1, 5, 45.0),
    ('Madcow 5x5','Workout A','Deadlift',       3, 1, 5, 95.0),
    ('Madcow 5x5','Workout B','Squat',          1, 1, 3, 45.0),
    ('Madcow 5x5','Workout B','Bench Press',    2, 1, 3, 45.0),
    ('Madcow 5x5','Workout B','Barbell Row',    3, 1, 3, 65.0)
) AS v(program, day, exercise, position, sets, reps, starting_weight_lb)
JOIN programs p      ON p.name = v.program
JOIN program_days pd ON pd.program_id = p.id AND pd.name = v.day
JOIN exercises e     ON e.name = v.exercise
ON CONFLICT (program_day_id, exercise_id) DO NOTHING;

COMMIT;
