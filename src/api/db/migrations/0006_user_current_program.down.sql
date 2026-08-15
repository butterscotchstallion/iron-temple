BEGIN;

ALTER TABLE users DROP COLUMN current_program_id;

COMMIT;
