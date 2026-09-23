BEGIN;

-- Reverses 0029. Lossy, and worth reading before running it on a live
-- deployment: every program a lifter built for themselves is deleted, along
-- with its days and its prescriptions. What is NOT deleted is logged work —
-- sessions and session_sets are untouched, so the sets, the tonnage and the
-- records stay, and the Racked recap still counts them.
--
-- Deleting them is the least bad option rather than a good one. The alternative
-- is to drop created_by_user_id and leave the rows, which would turn every
-- lifter's private program into a CANONICAL SEEDED ONE: shared with the whole
-- install, uneditable by its author, and indistinguishable from StrongLifts to
-- every query in the app. A migration that silently publishes private data is
-- worse than one that removes it.

-- ---------------------------------------------------------------------------
-- 1. Remove custom programs, except any that have been trained.
-- ---------------------------------------------------------------------------
--
-- A program whose days have sessions against them stays, for the reason 0009's
-- down gives about exercises with logged sets: sessions.program_day_id has no ON
-- DELETE clause, so the delete would fail outright and take the whole migration
-- with it. Leaving a few rows behind beats refusing to run.
--
-- The survivors become seeded programs when the column goes. That is the
-- publishing problem above, and it is unavoidable here — a program somebody has
-- trained cannot be removed and cannot keep an owner column that no longer
-- exists. It is called out rather than hidden: after running this, audit any
-- program that is not one of the eight seeded names.
--
-- program_days and program_day_exercises go with them via ON DELETE CASCADE, and
-- users.current_program_id clears itself via ON DELETE SET NULL.
DELETE FROM programs p
WHERE p.created_by_user_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1
      FROM sessions s
      JOIN program_days pd ON pd.id = s.program_day_id
      WHERE pd.program_id = p.id
  );

-- ---------------------------------------------------------------------------
-- 2. Restore program_days' plain uniques.
-- ---------------------------------------------------------------------------
--
-- Archived days that nobody trained go first. They are invisible to the app and
-- exist only to hold a name and a position out of reach, so there is nothing to
-- preserve — and every one left behind is a chance for the constraints below to
-- collide with a live day of the same name or position.
DELETE FROM program_days pd
WHERE pd.archived_at IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM sessions s WHERE s.program_day_id = pd.id);

DROP INDEX IF EXISTS program_days_position_live_idx;
DROP INDEX IF EXISTS program_days_name_live_idx;

-- These two can fail, and this is the likeliest place the migration stops. An
-- archived day that HAS been trained survived the delete above, and if it shares
-- a name or a position with a live day in the same program — which archiving
-- made legal — the constraint cannot be restored. The whole migration then rolls
-- back, leaving 0029 applied. Rename or renumber the offending day and run it
-- again.
--
-- Restored under the names Postgres gave them in 0001, so re-applying 0029 finds
-- the constraints it expects to drop.
ALTER TABLE program_days
    ADD CONSTRAINT program_days_program_id_name_key UNIQUE (program_id, name);
ALTER TABLE program_days
    ADD CONSTRAINT program_days_program_id_position_key UNIQUE (program_id, position);

ALTER TABLE program_days DROP COLUMN archived_at;

-- ---------------------------------------------------------------------------
-- 3. Restore programs' global unique name.
-- ---------------------------------------------------------------------------

DROP INDEX IF EXISTS programs_owner_idx;
DROP INDEX IF EXISTS programs_name_owned_idx;
DROP INDEX IF EXISTS programs_name_shared_idx;

ALTER TABLE programs
    DROP COLUMN archived_at,
    DROP COLUMN is_shared,
    DROP COLUMN created_by_user_id;

-- Can fail for the same shape of reason as above: a trained custom program
-- survived step 1 and its name collides case-sensitively with another survivor
-- or with a seeded program. Rename it and re-run.
ALTER TABLE programs ADD CONSTRAINT programs_name_key UNIQUE (name);

COMMIT;
