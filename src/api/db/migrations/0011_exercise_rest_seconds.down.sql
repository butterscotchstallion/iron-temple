BEGIN;

-- Dropping the column drops the tiers with it; the API falls back to the fixed
-- three minutes it used before, which is what every lift read as anyway.
ALTER TABLE exercises DROP COLUMN rest_seconds;

COMMIT;
