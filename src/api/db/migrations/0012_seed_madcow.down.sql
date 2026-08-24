BEGIN;

DELETE FROM program_day_exercises
 WHERE program_day_id IN (
     SELECT pd.id
     FROM program_days pd
     JOIN programs p ON p.id = pd.program_id
     WHERE p.name = 'Madcow 5x5');

DELETE FROM program_days
 WHERE program_id IN (SELECT id FROM programs WHERE name = 'Madcow 5x5');

DELETE FROM programs WHERE name = 'Madcow 5x5';

-- No exercise deletions: 0011 introduced none, and every lift it prescribed
-- belongs to the programs 0002 seeded.

COMMIT;
