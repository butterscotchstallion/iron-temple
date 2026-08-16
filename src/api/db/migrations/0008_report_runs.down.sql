-- Dropping the table takes its index with it.
--
-- This loses the record of which recaps were sent, so re-applying 0008 leaves
-- every completed period inside the reporter's catch-up window looking
-- outstanding. That is the intended shape of the down migration — the table is
-- the state — but it is worth knowing before running it on a live deployment.
DROP TABLE IF EXISTS report_runs;
