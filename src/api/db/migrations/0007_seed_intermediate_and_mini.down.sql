BEGIN;

DELETE FROM program_day_exercises
 WHERE program_day_id IN (
     SELECT pd.id
     FROM program_days pd
     JOIN programs p ON p.id = pd.program_id
     WHERE p.name IN ('StrongLifts 5x5 Intermediate', 'StrongLifts 5x5 Mini'));

DELETE FROM program_days
 WHERE program_id IN (
     SELECT id FROM programs
     WHERE name IN ('StrongLifts 5x5 Intermediate', 'StrongLifts 5x5 Mini'));

DELETE FROM programs
 WHERE name IN ('StrongLifts 5x5 Intermediate', 'StrongLifts 5x5 Mini');

-- Only the lifts 0007 introduced; the 0002 five stay for the programs that still use them.
DELETE FROM exercises
 WHERE name IN ('Pause Squat', 'Pause Bench Press', 'Pause Deadlift',
                'Incline Bench Press', 'Feet-Up Bench Press');

COMMIT;
