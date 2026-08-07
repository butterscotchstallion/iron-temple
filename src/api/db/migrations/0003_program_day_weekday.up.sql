BEGIN;

-- Optional weekday a program day is scheduled on (0 = Sunday … 6 = Saturday).
ALTER TABLE program_days
    ADD COLUMN weekday INTEGER
    CHECK (weekday >= 0 AND weekday <= 6);

COMMIT;
