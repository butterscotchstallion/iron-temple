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
-- an account created as "Judi.Bench" is found by "judi.bench".
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

-- ListGeneratedActivityCandidates finds the accounts a teardown MIGHT remove, and
-- returns the one piece of evidence that decides whether it should.
--
-- THE SECOND READER OF password_hash IN THIS DIRECTORY, and the exception is
-- deliberate. The rule at the top of users.sql exists so a hash cannot reach a
-- DTO — the row struct having no such field is what enforces it. This query does
-- not serve a DTO: the handler verifies each hash in process, uses the answer as a
-- boolean and discards it. Nothing derived from it is returned, so the property
-- that rule protects is intact.
--
-- WHY THE HASH IS THE RIGHT EVIDENCE
--
-- Generated accounts carry no marker — nothing in the schema and nothing on the
-- wire says they were not typed in by hand — which is a decision the install's
-- owner made. Teardown therefore used to match on USERNAME alone, and that was a
-- real hazard rather than a pedantic one: the personas are named from the same pun
-- list the admin area suggests usernames from, so an account the owner created by
-- hand as "judi.bench" — a name that form may well have offered them — was
-- indistinguishable from a generated one and would have been deleted along with
-- everything that lifter ever logged.
--
-- Every generated account is created with one fixed password that nothing ever
-- signs in with (generatedPassword in internal/api). That makes the stored hash a
-- proof of origin only the generator could have produced — already in the
-- database, needing no column, and revealing nothing on the wire. A real lifter
-- who happens to share a name chose their own password, so their hash does not
-- verify and they survive.
--
-- Scoped to non-admins here as well as at the delete. The owner is an admin and
-- everything they have lifted cascades from their row, so a name collision with
-- theirs must not even become a candidate.
-- name: ListGeneratedActivityCandidates :many
SELECT id, username, password_hash
FROM users
WHERE lower(username) = ANY(sqlc.arg('usernames')::text[])
  AND NOT is_admin;

-- DeleteGeneratedActivityUsers removes the accounts that actually proved out.
--
-- By id rather than by name, because the decision has already been made: the
-- caller verified each candidate's hash and this deletes exactly that set. Taking
-- names again would reopen the collision the verification just closed.
--
-- NOT is_admin is repeated anyway. It is redundant against a caller that only ever
-- passes ids from the query above, and it is the one predicate with no undo — an
-- admin's row cascades their entire training history — so it is not removable on
-- the grounds of being redundant.
--
-- Everything else about the account does cascade, by design: sessions, sets,
-- reactions, comments, gym, avatars. That is the point of a teardown.
--
-- :execrows so the caller can report what actually went rather than what it asked
-- for.
-- name: DeleteGeneratedActivityUsers :execrows
DELETE FROM users
WHERE id = ANY(sqlc.arg('ids')::int[])
  AND NOT is_admin;

-- ---- the daily schedule ----

-- GetActivitySchedule reads the singleton settings row.
--
-- :one with no argument and no empty case to handle: 0025 seeds the row, and the
-- CHECK (id = 1) means there can never be a second. So every caller gets a
-- schedule rather than a "not configured yet" branch.
-- name: GetActivitySchedule :one
SELECT id, enabled, lifters, updated_at FROM generated_activity_schedule WHERE id = 1;

-- SetActivitySchedule replaces it.
--
-- An UPDATE rather than an upsert for the same reason: the row is guaranteed to
-- exist, so an ON CONFLICT clause would be dead code that implied otherwise.
-- name: SetActivitySchedule :one
UPDATE generated_activity_schedule
SET enabled    = sqlc.arg('enabled'),
    lifters    = sqlc.arg('lifters')::int,
    updated_at = now()
WHERE id = 1
RETURNING id, enabled, lifters, updated_at;

-- ClaimActivityDay takes ownership of one day's generation, or reports that
-- somebody already has it.
--
-- THE WHOLE CONCURRENCY STORY IS THIS ONE STATEMENT. day is the primary key, so
-- ON CONFLICT DO NOTHING means exactly one caller can insert a given day: the
-- winner gets a row back, the losers get none and do nothing. That is what makes
-- the scheduler safe to run in more than one replica without any counting of
-- processes, and it is report_runs' argument applied to a simpler problem.
--
-- Claiming BEFORE generating rather than recording after is deliberate. Recording
-- afterwards would let two replicas both generate the same day and only then
-- discover the collision, by which point the duplicate sessions exist.
--
-- CALLED INSIDE THE SAME TRANSACTION AS THE GENERATION, which is what makes that
-- safe: a failed day rolls the claim back with everything else, so an aborted day is
-- indistinguishable from one nobody has touched and the retry is clean rather than
-- additive. There is deliberately no release query — a rollback is the release.
--
-- It also sharpens the race rather than blunting it. Two replicas attempting the same
-- day both reach this statement; the second blocks on the primary key until the first
-- commits or aborts, then either gets no row (already done) or takes the day itself.
-- No window exists in which a day is claimed but abandoned.
--
-- The counts go in as zero and are filled in by FinishActivityDay once the work is
-- known, so a claim carries no figures it has not earned.
-- name: ClaimActivityDay :one
INSERT INTO generated_activity_runs (day)
VALUES (sqlc.arg('day'))
ON CONFLICT (day) DO NOTHING
RETURNING day, sessions, reactions, comments, generated_at;

-- FinishActivityDay records what a claimed day actually produced.
-- name: FinishActivityDay :exec
UPDATE generated_activity_runs
SET sessions     = sqlc.arg('sessions')::int,
    reactions    = sqlc.arg('reactions')::int,
    comments     = sqlc.arg('comments')::int,
    generated_at = now()
WHERE day = sqlc.arg('day');

-- LastActivityRun is the most recent day that was generated, for the admin screen.
--
-- Ordered by day rather than by generated_at: a catch-up writes several days in one
-- pass with nearly identical timestamps, and what an operator wants to know is how
-- far the activity reaches, not which row was written last.
-- name: LastActivityRun :one
SELECT day, sessions, reactions, comments, generated_at
FROM generated_activity_runs
ORDER BY day DESC
LIMIT 1;

-- ListSessionsCommentedOnBy is the sessions one lifter has already commented on.
--
-- The generator's guard against saying something twice about the same workout. There
-- is deliberately NO uniqueness constraint on session_comments — a real lifter
-- replying to a thread on their own session is legitimate, and the app must allow it
-- — so "one comment per lifter per session" is a property of the GENERATOR rather
-- than of the schema, and this is how it enforces it.
--
-- Without it, recognition running once per generated day meant a catch-up could stack
-- several comments from the same persona onto the same older session in a single tick.
-- The window on recent sessions bounds how many passes see a session; this bounds it
-- to one regardless.
--
-- Read once per persona per pass rather than checked per candidate: a set in memory is
-- cheaper than a query per session, and the answer cannot change underneath a pass
-- that holds the only transaction writing it.
-- name: ListSessionsCommentedOnBy :many
SELECT DISTINCT session_id FROM session_comments WHERE user_id = sqlc.arg('user_id')::int;

-- ListMentionableSessionExercises is the lifts in one session that a persona may
-- name out loud in a comment.
--
-- Two filters, and both of them are the point.
--
-- created_by_user_id IS NULL keeps it to the SHARED library. A custom exercise
-- belongs to the lifter who made it, and ListExercises hides other people's — so a
-- persona saying "good work on the Zercher shrug" would publish a name the rest of
-- the install has no access to, which is the leak the feed's program masking already
-- exists to prevent. Same class of bug, one surface further on: a generated comment
-- is readable by everybody.
--
-- actual_reps > 0 keeps it to lifts actually PERFORMED. A session carries a set per
-- prescribed lift from the moment it is created, so without this a persona could
-- admire work the lifter skipped — the kind of disagreement with the session it hangs
-- off that makes the simulation obvious.
--
-- Unscoped by viewer, unlike most reads here, because there is nothing left to scope:
-- what survives both filters is a global exercise name in a session already listed to
-- this persona by ListFeedSessions.
--
-- DISTINCT and ordered by name: a lift with five sets is one thing to talk about, and
-- a stable order keeps a run reproducible from its seed.
-- name: ListMentionableSessionExercises :many
SELECT DISTINCT e.name
FROM session_sets ss
JOIN exercises e ON e.id = ss.exercise_id
WHERE ss.session_id = sqlc.arg('session_id')::int
  AND ss.actual_reps > 0
  AND e.created_by_user_id IS NULL
ORDER BY e.name;
