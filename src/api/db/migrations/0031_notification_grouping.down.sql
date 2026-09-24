BEGIN;

-- Undoes 0031 by putting 0030's index back in its original shape, which is the
-- whole of it: nothing was added to the table on the way up, so nothing is lost
-- on the way down.
--
-- Rolling back the index alone is safe in a way a rollback usually is not. The
-- grouped count still WORKS against the narrower index — it just reads the heap
-- to group what it found instead of reading the groups straight out of the
-- index. So an install rolled back to 0030 while still running this version of
-- the code gets a slower badge and a correct one, and an install rolled back
-- past the code too gets exactly what it had before.

DROP INDEX IF EXISTS notifications_unread_idx;

-- 0030's version, restored exactly.
CREATE INDEX notifications_unread_idx
    ON notifications (user_id)
    WHERE read_at IS NULL AND archived_at IS NULL;

COMMIT;
