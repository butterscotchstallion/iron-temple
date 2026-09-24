BEGIN;

-- WHAT THIS LOSES, said plainly: the history.
--
-- The catalogue comes back on the way up, because the up migration seeds it, and
-- the current reigns come back within the hour because the reconciler recomputes
-- them from the standings. Neither is a reason to hesitate.
--
-- The ledger of PAST reigns is different. It is the one thing in this feature
-- that is not derivable from anything else — nothing else in the database records
-- who was top of a board in July — so going back down and up again leaves every
-- lifter's "held this three times" reading "held this once, since the redeploy".
-- That is a real loss and it is accepted here for the same reason 0026 accepted
-- losing read state: this is a down migration, it runs when a deploy is being
-- undone, and refusing to drop would leave a table nothing reads.
--
-- Order matters below. The notifications column references achievements(slug),
-- so it goes first; lifter_achievements references both users and achievements,
-- so it goes before the catalogue. Dropping in the wrong order is a constraint
-- error rather than a silent problem, but spelling it out is cheaper than
-- rediscovering it during a rollback.

ALTER TABLE notifications DROP COLUMN IF EXISTS achievement_slug;

DROP TABLE IF EXISTS lifter_achievements;
DROP TABLE IF EXISTS achievements;

COMMIT;
