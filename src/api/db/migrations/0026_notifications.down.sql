BEGIN;

-- Drops unconditionally, like 0024 and 0025 and unlike the program
-- down-migrations.
--
-- Nothing here is irrecoverable. Every notification was derived from an event
-- that outlives it — the reaction, the comment, the account — and the up
-- migration's backfill rebuilds the whole table from those tables on the way
-- back up. What is genuinely lost is read state: which of them had been seen.
-- That is one tap to restore and not a reason to refuse.
--
-- The indexes go with the table; naming them here would be noise.

DROP TABLE IF EXISTS notifications;

COMMIT;
