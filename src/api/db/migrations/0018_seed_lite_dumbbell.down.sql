BEGIN;

DELETE FROM program_day_exercises
 WHERE program_day_id IN (
     SELECT pd.id
     FROM program_days pd
     JOIN programs p ON p.id = pd.program_id
     WHERE p.name = 'StrongLifts 5x5 Lite (Dumbbell Press)');

DELETE FROM program_days
 WHERE program_id IN (
     SELECT id FROM programs WHERE name = 'StrongLifts 5x5 Lite (Dumbbell Press)');

DELETE FROM programs WHERE name = 'StrongLifts 5x5 Lite (Dumbbell Press)';

-- The lift goes back to being an accessory, since nothing prescribes it again.
-- No exercise is deleted: 0018 introduced none, it only reclassified one that
-- 0009 seeded, and 0009's own down migration is what removes it.
UPDATE exercises
   SET is_accessory = true
 WHERE name = 'Dumbbell Shoulder Press'
   AND created_by_user_id IS NULL;

COMMIT;
