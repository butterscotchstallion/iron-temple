BEGIN;

-- Lossy, and the loss is the whole column: every weigh-in a lifter recorded goes
-- with it. Logged work is untouched — bodyweight was never an input to volume,
-- progression or the Racked recap.
--
-- Dropping the column takes sessions_bodyweight_idx with it; the index is partial
-- on this column and cannot outlive it.
ALTER TABLE sessions DROP COLUMN bodyweight_lb;

COMMIT;
