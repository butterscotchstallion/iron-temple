BEGIN;

-- Undoes 0030, and the one thing worth saying is what it does to rows that were
-- archived.
--
-- It brings them BACK. Dropping archived_at loses the only mark distinguishing
-- an archived notification from a live one, so every one of them reappears in
-- its owner's panel — and any that were still unread reappear in the badge.
--
-- That is lossy in the direction that is merely noisy rather than destructive,
-- which is the right way round: nothing is deleted, and a lifter on a rolled-back
-- install sees a long panel and taps "mark all read" once. The alternative —
-- deleting the archived rows on the way down so the panel looks the same —
-- would make a rollback destroy data, which is exactly what a rollback must not
-- do.
--
-- The indexes go first. Both reference the column, so neither can outlive it,
-- and 0026's notifications_unread_idx has to be put back in its original shape
-- because 0030 replaced it rather than adding alongside it.

DROP INDEX IF EXISTS notifications_live_idx;
DROP INDEX IF EXISTS notifications_unread_idx;

ALTER TABLE notifications DROP COLUMN IF EXISTS archived_at;

-- 0026's version, restored exactly.
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;

COMMIT;
