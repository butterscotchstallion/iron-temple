-- Following, which decides whose achievements reach whose panel.
--
-- Two writes and no reads. That is not an omission: nothing asks this table a
-- question on its own. The roster and the profile test it with a correlated EXISTS
-- in users.sql, and the crown fan-out joins it in achievements.sql — both of which
-- keep the follow check next to the row it qualifies rather than making a caller
-- fetch a set and filter in Go.
--
-- Both writes are modelled on AddSessionReaction and RemoveSessionReaction in
-- recognition.sql, down to the reasoning: a button that toggles a state should
-- report the state, not complain about the number of times it was pressed.

-- FollowLifter records that one lifter wants to hear about another.
--
-- ON CONFLICT DO NOTHING rather than an error, for AddSessionReaction's reason: the
-- primary key makes a repeat press a no-op, which is what pressing Follow twice
-- means. Returning a 409 would be reporting a mistake nobody made, and checking
-- first would be a race under concurrent presses that the constraint settles for
-- free.
--
-- :execrows so a caller CAN tell a new follow from a repeat, even though the
-- handler answers 204 either way. Nothing needs the distinction today — unlike
-- applause, a follow raises no notification — and the row count is there because
-- the day it does, the alternative is a read-then-write race.
--
-- Following yourself cannot be written: 0033's CHECK refuses it, and the handler
-- refuses it first with a message worth reading.
-- name: FollowLifter :execrows
INSERT INTO follows (followee_id, follower_id)
VALUES (sqlc.arg('followee_id')::int, sqlc.arg('follower_id')::int)
ON CONFLICT (followee_id, follower_id) DO NOTHING;

-- UnfollowLifter stops it.
--
-- :execrows to match its sibling, and the handler discards the count for
-- RemoveSessionReaction's reason: the caller asked for a state — "I do not follow
-- this lifter" — and that state holds whether or not a row went. Reporting 404 for
-- a follow already gone would turn a double-tap into an error.
-- name: UnfollowLifter :execrows
DELETE FROM follows
WHERE followee_id = sqlc.arg('followee_id')::int
  AND follower_id = sqlc.arg('follower_id')::int;
