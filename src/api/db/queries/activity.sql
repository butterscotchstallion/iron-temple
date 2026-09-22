-- Queries that exist only to generate activity on an install that needs some.
--
-- EVERY QUERY IN THIS FILE IS REACHABLE ONLY FROM /admin/activity, and none of
-- them belongs on a lifter-facing path. Two of them can do things no ordinary
-- request may: rewrite a session's clock, and delete accounts. They are separated
-- into their own file so that is visible from the filename rather than discovered
-- by reading a handler.
--
-- If a lifter-facing feature ever seems to want one of these, that is the signal
-- to write a different query rather than to widen this one.

-- FindUserIDByUsername resolves an account by name, case-insensitively, to nothing
-- but its id.
--
-- Exists because the backfill has to be re-runnable: it derives the accounts it
-- wants from a fixed roster, and needs to tell "this one is already here" from
-- "create it" without going through GetUserForLogin, which is the one query
-- allowed to read a password hash and must not be called for anything else.
--
-- Matched on lower(username) so it agrees with the unique index and with login —
-- an account created as "Mara.Quinn" is found by "mara.quinn".
-- name: FindUserIDByUsername :one
SELECT id FROM users WHERE lower(username) = lower(sqlc.arg('username'));

-- CreateLoggedSessionSet inserts a set that has already been performed.
--
-- The app never does this, and the asymmetry is the reason this query is here
-- rather than in sessions.sql: a real session MATERIALISES its sets from the
-- prescription (CreateSessionSet) and the lifter logs them one at a time
-- afterwards (UpdateSessionSet), because that is the order those things happen in
-- at a rack.
--
-- Generated history has no such order to respect, and going through the two-step
-- would be an insert plus an update per set for the same end state. It would also
-- be the more dangerous route: UpdateSessionSet writes all three mutable columns
-- on purpose — it is the only way a PATCH can clear actual_reps back to NULL — so
-- calling it to set reps means restating the weight, and a caller that forgot
-- would silently zero the load on every set it touched.
--
-- Columns and their meanings are sessions.sql's: target_reps is what was asked
-- for, actual_reps what was done, and completed is the lifter's tick rather than
-- anything derived — the caller decides it, exactly as a lifter would.
-- name: CreateLoggedSessionSet :exec
INSERT INTO session_sets
    (session_id, exercise_id, set_number, target_reps, weight_lb, actual_reps, completed)
VALUES (
    sqlc.arg('session_id')::int,
    sqlc.arg('exercise_id')::int,
    sqlc.arg('set_number')::int,
    sqlc.arg('target_reps')::int,
    sqlc.arg('weight_lb'),
    sqlc.arg('actual_reps')::int,
    sqlc.arg('completed')
);

-- SetSessionClock rewrites when a session started and finished.
--
-- Nothing a lifter does can reach this, and nothing should: created_at is the
-- moment the session was materialised and finished_at the moment they tapped
-- Finish, and both are facts rather than fields.
--
-- Generated history needs it anyway. performed_on can be backdated through the
-- ordinary create path, but duration is finished_at - created_at — so a session
-- backdated three months and materialised a second ago reads as having taken no
-- time at all. That is not a cosmetic problem: session pace, the "fastest
-- session" highlight and the whole ranking of a workout against its own history
-- are computed from duration, and an install full of zero-second sessions shows
-- those features working on nonsense.
--
-- Scoped by user_id as well as id. The caller already knows whose session it is,
-- so the predicate is redundant to it and is there for the next caller — this is
-- the most dangerous query in the directory and it should be hard to point at a
-- row by accident.
-- name: SetSessionClock :exec
UPDATE sessions
SET created_at  = sqlc.arg('started_at'),
    finished_at = sqlc.arg('finished_at')
WHERE id = sqlc.arg('id')
  AND user_id = sqlc.arg('user_id')::int;

-- SetProgramDayWeekdayIfUnset schedules a program day, but only if nobody has.
--
-- Attendance needs a schedule to grade against: without a weekday on the day,
-- racked reports AttendanceNone and the leaderboard's attendance board has
-- nothing to rank. So generated lifters need their program days scheduled.
--
-- The IF UNSET is the whole point. program_days is SHARED — one row per day per
-- program, for the entire install — so writing a weekday here changes what the
-- install's real owner sees on their own program page. Refusing to overwrite an
-- existing value means generated activity can fill in a blank but can never
-- rearrange somebody's actual training week.
--
-- Not UpdateProgramDayWeekday, which sets unconditionally because it serves a
-- lifter deliberately changing their own schedule. That is the right behaviour
-- there and the wrong one here.
-- name: SetProgramDayWeekdayIfUnset :exec
UPDATE program_days
SET weekday = sqlc.narg('weekday')
WHERE id = sqlc.arg('id')
  AND weekday IS NULL;

-- DeleteGeneratedUsers removes accounts by name, refusing to touch an admin.
--
-- The teardown for a backfill. Generated accounts carry no marker — nothing in
-- the schema or on the wire says they were not typed in by hand — so the only
-- way to find them again is to re-derive the roster that named them and delete
-- those. That is why the roster has to be stable, and it is asserted in
-- internal/activity's tests for exactly this reason.
--
-- NOT is_admin is a guard against the one mistake with no undo. The install's
-- owner is an admin and everything they have ever lifted cascades from their row;
-- if a roster name ever collided with theirs, this predicate is what stands
-- between a teardown and somebody's training history. It costs nothing and it is
-- not removable.
--
-- Everything else about the account does cascade, by design: sessions, sets,
-- reactions, comments, gym, avatars. That is the point of a teardown.
--
-- :execrows so the caller can report what actually went, rather than what it
-- asked for.
-- name: DeleteGeneratedUsers :execrows
DELETE FROM users
WHERE lower(username) = ANY(sqlc.arg('usernames')::text[])
  AND NOT is_admin;
