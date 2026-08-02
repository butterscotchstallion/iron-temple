BEGIN;

DELETE FROM program_day_exercises
 WHERE program_day_id IN (
     SELECT pd.id
     FROM program_days pd
     JOIN programs p ON p.id = pd.program_id
     WHERE p.name IN ('StrongLifts 5x5', 'StrongLifts 5x5 Lite', 'Advanced 3x5'));

DELETE FROM program_days
 WHERE program_id IN (
     SELECT id FROM programs
     WHERE name IN ('StrongLifts 5x5', 'StrongLifts 5x5 Lite', 'Advanced 3x5'));

DELETE FROM programs
 WHERE name IN ('StrongLifts 5x5', 'StrongLifts 5x5 Lite', 'Advanced 3x5');

DELETE FROM exercises
 WHERE name IN ('Squat', 'Bench Press', 'Barbell Row', 'Overhead Press', 'Deadlift');

COMMIT;
