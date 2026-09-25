BEGIN;

-- WHAT THIS LOSES, said plainly: every House, and every membership in one.
--
-- 0032's down migration could point at a reconciler that rebuilds the current
-- reigns within the hour, and lose only the history. This has no such comfort.
-- A House is not derived from anything — it was typed in by a lifter, named,
-- given a sigil and a tagline, and joined by people who asked and were let in.
-- Nothing recomputes that. Going down and back up leaves an install with no
-- Houses at all and no way to tell whose they were.
--
-- It is accepted for the reason every down migration accepts its loss: this runs
-- when a deploy is being undone, and the alternative is refusing to drop and
-- leaving three tables and a column that nothing reads. An operator who might
-- want the data back wants a dump, not a migration that declines to run.
--
-- Order matters below. notifications.house_id references houses(id), so the
-- column goes first. house_members and house_join_requests both reference
-- houses(id), so they go before it. Dropping in the wrong order is a constraint
-- error rather than a silent problem, but spelling it out is cheaper than
-- rediscovering it halfway through a rollback.

ALTER TABLE notifications DROP COLUMN IF EXISTS house_id;

DROP TABLE IF EXISTS house_join_requests;
DROP TABLE IF EXISTS house_members;
DROP TABLE IF EXISTS houses;

COMMIT;
