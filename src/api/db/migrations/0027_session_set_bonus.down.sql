BEGIN;

-- Lossy, unlike 0016's down: this column holds a judgement made at the moment a
-- set was appended, and nothing else in the schema records the inputs to it.
-- Dropping it does not send the reads back to computing the answer a slower way
-- — it destroys the answer. Rolling forward again starts the record over from
-- the sets appended after that point.
--
-- Everything that reads it degrades to "no bonus sets", which is the same
-- reading every pre-0027 session already gives, so the app stays correct while
-- saying less.
ALTER TABLE session_sets
    DROP COLUMN is_bonus;

COMMIT;
