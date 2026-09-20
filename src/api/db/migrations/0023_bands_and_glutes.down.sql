BEGIN;

-- Reverses 0023, on the condition that nobody has trained the program.
--
-- sessions.program_day_id references program_days WITHOUT an ON DELETE clause
-- (0001), so if a single session was performed against Workout A or B the
-- DELETE below raises a foreign key violation and the whole migration rolls
-- back, leaving 0023 applied. That is the right outcome rather than a bug to
-- work around: the alternative is deleting a lifter's sessions to tidy up a
-- program, and logged work is the one thing in this database that cannot be
-- recomputed. Every program down migration since 0002 has this property; 0023
-- is the first to write it down.
--
-- Per-account assistance bolted onto these days does cascade, by way of
-- program_day_assistance.program_day_id (0009), and is lost. Those rows are a
-- prescription rather than a record — what was actually lifted is in
-- session_sets, which nothing here touches.

DELETE FROM program_day_exercises
 WHERE program_day_id IN (
     SELECT pd.id
     FROM program_days pd
     JOIN programs p ON p.id = pd.program_id
     WHERE p.name = 'Glutes & Legs (Bands and Free Weights)');

DELETE FROM program_days
 WHERE program_id IN (
     SELECT id FROM programs WHERE name = 'Glutes & Legs (Bands and Free Weights)');

DELETE FROM programs WHERE name = 'Glutes & Legs (Bands and Free Weights)';

-- The five promoted lifts go back to being accessories, since nothing
-- prescribes them again. None is deleted: 0023 introduced none of them, it only
-- reclassified rows 0009 seeded, and 0009's own down migration is what removes
-- those.
UPDATE exercises
   SET is_accessory = true
 WHERE name IN (
    'Front Squat',
    'Bulgarian Split Squat',
    'Romanian Deadlift',
    'Walking Lunge',
    'Back Extension'
 )
   AND created_by_user_id IS NULL;

-- The five movements 0023 did introduce, removed — but only where nothing
-- performed them. Same rule and same reason as 0009's down migration:
-- session_sets.exercise_id has no ON DELETE clause, so deleting a lift with
-- logged sets against it would fail outright, and a history that quietly lost
-- its lifts would be worse than a catalogue that keeps a few rows.
DELETE FROM exercises e
 WHERE e.name IN (
    'Barbell Hip Thrust',
    'Banded Lateral Walk',
    'Banded Hip Abduction',
    'Banded Glute Bridge',
    'Banded Kickback'
 )
   AND e.created_by_user_id IS NULL
   AND NOT EXISTS (SELECT 1 FROM session_sets ss WHERE ss.exercise_id = e.id);

-- Narrow the equipment CHECK back to 0009's six kinds, under the name Postgres
-- gave the inline constraint so re-applying 0023 finds what it expects to drop.
--
-- This is the one statement that can fail, and the failure is informative
-- rather than mysterious: it means a banded movement survived the DELETE above
-- because it has logged sets, so there is a row reading equipment = 'band' that
-- the narrowed constraint will not admit. The whole migration rolls back,
-- leaving 0023 applied. Reclassify the surviving rows to 'other' and run it
-- again — that is the bucket 0009 keeps for equipment the app does not model,
-- and it is where band work would have lived had this migration never existed.
ALTER TABLE exercises
    DROP CONSTRAINT exercises_equipment_check;
ALTER TABLE exercises
    ADD CONSTRAINT exercises_equipment_check
        CHECK (equipment IN ('barbell', 'dumbbell', 'machine', 'cable',
                             'bodyweight', 'other'));

COMMIT;
