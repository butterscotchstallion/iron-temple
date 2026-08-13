BEGIN;

ALTER TABLE sessions DROP COLUMN finished_at;

COMMIT;
