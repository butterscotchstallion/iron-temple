BEGIN;

-- Notifications stop growing forever.
--
-- 0026 built this table and left it unbounded on purpose: rows are only written
-- by live events, "clear all" deletes, and on a household install that seemed
-- like enough. It is not, and the reason is the generated-activity scheduler
-- (0025) — it applauds and comments as its personas every single day, and every
-- one of those raises a row in a real lifter's panel. An install left running
-- accumulates notifications at a steady rate forever, and nothing ever reads the
-- old ones: the panel holds twenty and there is no "load more" behind it.
--
-- THIS IS NOT THE cleared_at 0026 REJECTED, and the distinction matters enough
-- to spell out, because the two would look identical in a schema dump.
--
-- 0026 argued against a soft delete for CLEARING, and that argument still holds
-- and is untouched here: "clear all" is still a DELETE, the rows still go, and a
-- cleared notification still stays gone. What it was protecting was the
-- independence of read and cleared — a watermark can express only one of them —
-- and nothing below touches either.
--
-- archived_at answers a different question. It is not something a lifter does;
-- it is something time does, applied by a sweeper to rows nobody can reach any
-- more. Keeping them rather than deleting them means the history of what
-- happened on this install is still in the database to be queried, which a
-- DELETE would throw away for no gain — the cost of an old row is an index
-- entry, and the partial indexes below mean it is not even that.
--
-- So: a lifter clears their panel and the rows are gone. Time passes and a row
-- is archived and is merely out of sight. Those are different enough to be
-- different mechanisms.

ALTER TABLE notifications ADD COLUMN archived_at TIMESTAMPTZ;

-- The panel's read, now that it has two predicates.
--
-- notifications_user_idx (0026) leads on user_id and carries both sort columns,
-- but knows nothing about archived_at, so it would serve the scan and then
-- filter — fine while nothing is archived, and progressively less so as the
-- archive becomes most of the table, which is the steady state this migration
-- creates.
--
-- Partial on the live rows, which is the same shape 0029 used for
-- program_days_position_live_idx: the index holds only what anything ever reads,
-- and an archived row leaves it entirely rather than sitting in it forever.
CREATE INDEX notifications_live_idx
    ON notifications (user_id, created_at DESC, id DESC)
    WHERE archived_at IS NULL;

-- 0026's notifications_user_idx is deliberately KEPT rather than dropped. It is
-- the only index that can answer a question about an archived row, which is the
-- whole point of keeping them — and on the live rows the planner has the
-- narrower partial index above to prefer.

-- The badge's read.
--
-- 0026's version was partial on read_at alone. The count now excludes archived
-- rows too, so the old index would have stopped covering it — replaced rather
-- than added to, because an unread row that has been archived is not something
-- the badge counts and therefore not something this index needs to hold.
DROP INDEX IF EXISTS notifications_unread_idx;
CREATE INDEX notifications_unread_idx
    ON notifications (user_id)
    WHERE read_at IS NULL AND archived_at IS NULL;

-- No backfill. Every existing row is live, which is what a NULL archived_at
-- already says, and the sweeper will take the old ones on its next pass — within
-- the hour, and without this migration having to guess the retention window.

COMMIT;
