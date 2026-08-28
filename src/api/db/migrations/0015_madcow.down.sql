BEGIN;

-- Back to 0012's two-day A/B on the linear engine. The set plans go with the
-- table; every other program never had rows in it, so nothing else notices.
DROP TABLE program_day_exercise_sets;

DELETE FROM program_days
WHERE program_id = (SELECT id FROM programs WHERE name = 'Madcow 5x5');

UPDATE programs
SET progression_kind = 'linear',
    description = 'Two-day A/B alternation: fives on squat, press and deadlift, then a day of heavy triples.'
WHERE name = 'Madcow 5x5';

INSERT INTO program_days (program_id, name, position)
SELECT p.id, v.name, v.position
FROM programs p
CROSS JOIN (VALUES ('Workout A', 1), ('Workout B', 2)) AS v(name, position)
WHERE p.name = 'Madcow 5x5';

INSERT INTO program_day_exercises
    (program_day_id, exercise_id, position, sets, reps, starting_weight_lb)
SELECT pd.id, e.id, v.position, v.sets, v.reps, v.starting_weight_lb
FROM (VALUES
    ('Workout A', 'Squat',          1, 2, 5, 45.0),
    ('Workout A', 'Overhead Press', 2, 1, 5, 45.0),
    ('Workout A', 'Deadlift',       3, 1, 5, 95.0),
    ('Workout B', 'Squat',          1, 1, 3, 45.0),
    ('Workout B', 'Bench Press',    2, 1, 3, 45.0),
    ('Workout B', 'Barbell Row',    3, 1, 3, 65.0)
) AS v(day, exercise, position, sets, reps, starting_weight_lb)
JOIN program_days pd
  ON pd.name = v.day
 AND pd.program_id = (SELECT id FROM programs WHERE name = 'Madcow 5x5')
JOIN exercises e ON e.name = v.exercise AND e.created_by_user_id IS NULL;

-- Restored last: the CHECK cannot be narrowed while a 'madcow' row exists.
ALTER TABLE programs
    DROP CONSTRAINT programs_progression_kind_check;

ALTER TABLE programs
    ADD CONSTRAINT programs_progression_kind_check
    CHECK (progression_kind IN ('linear'));

COMMIT;
