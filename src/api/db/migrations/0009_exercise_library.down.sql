BEGIN;

-- Reverses 0009. This is lossy in a way worth reading before running it on a
-- live deployment: every lifter's assistance plan is deleted, and so is every
-- custom exercise they created, along with the library metadata on the rows that
-- survive. What is NOT deleted is logged work — session_sets is untouched, so
-- the sets and the tonnage stay, and the Racked recap still counts them.

-- The plans go first, which also releases the assistance table's references to
-- the exercises deleted below. Dropping the table takes its index with it.
DROP TABLE IF EXISTS program_day_assistance;

-- Remove the accessories this migration seeded and any custom exercises lifters
-- added, but only where nothing performed them. An exercise with logged sets
-- against it stays: session_sets.exercise_id has no ON DELETE clause, so
-- deleting it would fail outright, and a history that silently loses its lifts
-- would be worse than one that keeps a few rows this migration created.
DELETE FROM exercises e
WHERE (e.is_accessory OR e.created_by_user_id IS NOT NULL)
  AND NOT EXISTS (SELECT 1 FROM session_sets ss WHERE ss.exercise_id = e.id)
  AND NOT EXISTS (
      SELECT 1 FROM program_day_exercises pde WHERE pde.exercise_id = e.id
  );

DROP INDEX IF EXISTS exercises_owner_idx;
DROP INDEX IF EXISTS exercises_name_owned_idx;
DROP INDEX IF EXISTS exercises_name_shared_idx;

ALTER TABLE exercises
    DROP COLUMN created_by_user_id,
    DROP COLUMN is_accessory,
    DROP COLUMN equipment,
    DROP COLUMN muscle_group;

-- Restore the global unique name from 0001, under the name Postgres gave it so
-- re-applying 0009 finds the constraint it expects to drop.
--
-- This is the one statement that can fail. It does so only if a custom exercise
-- survived the DELETE above (it had logged sets) and its name collides
-- case-sensitively with another surviving row. The API refuses to create a
-- custom exercise whose name matches an existing one, so reaching that state
-- takes a direct write to the database. If it happens the whole migration rolls
-- back, leaving 0009 applied — rename the offending row and run it again.
ALTER TABLE exercises ADD CONSTRAINT exercises_name_key UNIQUE (name);

COMMIT;
