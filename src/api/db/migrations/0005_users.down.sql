BEGIN;

-- Drop the owner column first: the FK to users(id) has to go before the table
-- it points at.
DROP INDEX IF EXISTS sessions_user_idx;
ALTER TABLE sessions DROP COLUMN user_id;

DROP TABLE user_avatars;
DROP TABLE user_sessions;
DROP TABLE users;

COMMIT;
