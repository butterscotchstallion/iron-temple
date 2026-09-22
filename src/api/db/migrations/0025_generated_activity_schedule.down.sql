BEGIN;

-- Reverses 0025 unconditionally.
--
-- Both tables are bookkeeping about generation, not the generated data itself: the
-- sessions, reactions and comments a daily run produced belong to ordinary lifter
-- accounts and are removed the way any generated activity is, through
-- DELETE /admin/activity. Dropping these loses the schedule and the record of
-- which days ran, and touches no training history.
--
-- The consequence worth knowing: with the runs table gone, a later re-apply of
-- 0025 starts with nothing marked as done, so the first tick after that will
-- generate the whole catch-up window again. That is a duplicate day of activity
-- rather than a correctness problem, and it is the right trade for a down
-- migration that must not refuse.

DROP TABLE IF EXISTS generated_activity_runs;
DROP TABLE IF EXISTS generated_activity_schedule;

COMMIT;
