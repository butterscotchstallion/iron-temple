BEGIN;

-- Widen the rail before restoring the data that needs the room.
ALTER TABLE exercises
    DROP CONSTRAINT exercises_rest_seconds_check;
ALTER TABLE exercises
    ADD CONSTRAINT exercises_rest_seconds_check
        CHECK (rest_seconds BETWEEN 30 AND 900);

-- The up migration collapsed 300 into 180, which is lossy: afterwards a squat
-- and a bench press are indistinguishable by rest alone. So this restores 0011's
-- five-minute tier from 0011's name list rather than from anything in the table
-- — the same six lifts, back where they started.
--
-- A movement that was above 180 for some other reason when the up ran does not
-- come back. Nothing seeds one, and no endpoint writes the column, so today that
-- set is empty; if a later migration changes that, it owns its own down.
UPDATE exercises SET rest_seconds = 300
WHERE name IN (
    'Squat', 'Pause Squat', 'Front Squat',
    'Deadlift', 'Pause Deadlift', 'Romanian Deadlift'
);

COMMIT;
