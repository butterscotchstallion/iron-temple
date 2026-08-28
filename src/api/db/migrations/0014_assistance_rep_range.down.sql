BEGIN;

-- Dropping the range drops the progression with it: every assistance lift goes
-- back to carrying forward whatever was last logged, which is what they all did
-- before and what the ones without a range do now.
ALTER TABLE program_day_assistance
    DROP CONSTRAINT program_day_assistance_rep_range_ck;

ALTER TABLE program_day_assistance
    DROP COLUMN rep_min,
    DROP COLUMN rep_max;

COMMIT;
