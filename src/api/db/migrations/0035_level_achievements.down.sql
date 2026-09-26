BEGIN;

-- WHAT THIS LOSES, said plainly: which rung each lifter had reached, and when.
--
-- Less than 0032's down migration loses, and for one reason worth stating: unlike a
-- crown reign, a level reign is RE-DERIVABLE. Going back down and up again re-runs
-- the up migration's backfill, which reads the sessions and reopens a reign for
-- every lifter still past a threshold — so the holdings come back. What does not
-- come back is the DATE: every reign returns stamped with the moment of the
-- redeploy rather than the session that actually crossed it.
--
-- That is accepted for 0032's reason. This is a down migration; it runs when a
-- deploy is being undone, and refusing to drop would leave a column and four
-- catalogue rows that nothing reads.
--
-- Order matters. The reigns reference achievements(slug), so they go before the
-- catalogue rows they point at. Deleting the reigns explicitly rather than leaning
-- on the ON DELETE CASCADE from the catalogue, because the cascade would also be
-- the thing that silently did it — and a row count in the log is worth more during
-- a rollback than a constraint that happened to hold.
--
-- Scoped to kind = 'level' throughout. A DELETE that took the crowns with it would
-- destroy the one thing in this feature that genuinely is not derivable, on a
-- migration that has nothing to do with them.

DELETE FROM lifter_achievements
WHERE achievement_slug IN (SELECT slug FROM achievements WHERE kind = 'level');

DELETE FROM notifications
WHERE achievement_slug IN (SELECT slug FROM achievements WHERE kind = 'level');

DELETE FROM achievements WHERE kind = 'level';

ALTER TABLE achievements DROP COLUMN IF EXISTS level_threshold;

COMMIT;
