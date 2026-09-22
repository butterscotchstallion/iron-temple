BEGIN;

-- Reverses 0024 unconditionally, unlike the program down-migrations either side
-- of it.
--
-- Those refuse to run once anybody has trained, because they would have to delete
-- session_sets to succeed and logged work cannot be recomputed. Nothing here is
-- in that category: a reaction and a comment are things people said about a
-- workout, not the workout. Dropping them loses the conversation and leaves every
-- session, set and rep exactly as it was.
--
-- The indexes go with their tables; naming them here would be redundant.

DROP TABLE IF EXISTS session_comments;
DROP TABLE IF EXISTS session_reactions;

COMMIT;
