-- A session is "over" when the lifter finished it by hand, or when it was
-- started more than 12 hours ago and so cannot still be in progress. This is
-- derived at read time rather than materialized: no background job has to run,
-- and there is no window in which a stale row disagrees with the clock. The
-- expression is repeated in the queries below because a generated column cannot
-- use now(), which Postgres does not consider immutable. Keep the copies in
-- sync — GetSession, ListSessions and ListLiftHistory all depend on it.
--
--     s.finished_at IS NOT NULL OR s.created_at < now() - INTERVAL '12 hours'
--
-- Every query here is scoped to one owner (s.user_id = user_id). That filter is
-- the whole of the isolation model, so it is not optional on any read or write:
-- programs and exercises are shared, but performances are not. A session the
-- caller does not own must be indistinguishable from one that does not exist,
-- which is why the set-level queries below reach the owner through a JOIN back
-- to sessions rather than trusting the set id alone — a caller learning that
-- someone else's set id is valid is already a leak.

-- name: CreateSession :one
INSERT INTO sessions (program_day_id, performed_on, user_id)
VALUES (sqlc.arg('program_day_id'), sqlc.arg('performed_on'), sqlc.arg('user_id')::int)
RETURNING id, program_day_id, performed_on, notes, created_at, finished_at, user_id;

-- name: CreateSessionSet :one
INSERT INTO session_sets (session_id, exercise_id, set_number, target_reps, weight_lb)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, session_id, exercise_id, set_number, target_reps, actual_reps, weight_lb, completed;

-- name: GetSession :one
SELECT s.id,
       s.program_day_id,
       pd.name AS program_day_name,
       p.id    AS program_id,
       p.name  AS program_name,
       s.performed_on,
       s.notes,
       s.created_at,
       s.finished_at,
       (s.finished_at IS NOT NULL
        OR s.created_at < now() - INTERVAL '12 hours')::bool AS is_over
FROM sessions s
JOIN program_days pd ON pd.id = s.program_day_id
JOIN programs p ON p.id = pd.program_id
WHERE s.id = sqlc.arg('id')
  AND s.user_id = sqlc.arg('user_id')::int;

-- ListSessions returns paginated session summaries, most recent first,
-- optionally filtered to one program. Pass NULL program_id for all programs.
-- is_over reads s.finished_at/s.created_at under a GROUP BY: legal because the
-- grouping includes s.id, the primary key, which makes every other sessions
-- column functionally dependent on it.
--
-- volume_lb is the weight actually moved: actual_reps, not target_reps, and
-- every logged set rather than only the completed ones — a set that stopped at
-- 3 of 5 reps still moved the bar three times. SUM already skips the NULL
-- actual_reps of an unlogged set; COALESCE covers a session where none is
-- logged. The ::numeric cast is what types the column for sqlc.
-- name: ListSessions :many
SELECT s.id,
       s.program_day_id,
       pd.name AS program_day_name,
       p.id    AS program_id,
       p.name  AS program_name,
       s.performed_on,
       COUNT(ss.id)                              AS set_count,
       COUNT(ss.id) FILTER (WHERE ss.completed)  AS completed_set_count,
       COALESCE(SUM(ss.actual_reps * ss.weight_lb), 0)::numeric AS volume_lb,
       (s.finished_at IS NOT NULL
        OR s.created_at < now() - INTERVAL '12 hours')::bool AS is_over
FROM sessions s
JOIN program_days pd ON pd.id = s.program_day_id
JOIN programs p ON p.id = pd.program_id
LEFT JOIN session_sets ss ON ss.session_id = s.id
WHERE s.user_id = sqlc.arg('user_id')::int
  AND (sqlc.narg('program_id')::bigint IS NULL OR p.id = sqlc.narg('program_id'))
GROUP BY s.id, pd.name, p.id, p.name
-- Only sessions with at least one logged rep count as "started".
HAVING COUNT(ss.id) FILTER (WHERE ss.actual_reps > 0) > 0
ORDER BY s.performed_on DESC, s.id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- SessionTotals returns the two figures that describe a whole history rather
-- than one page of it: how many sessions match the filter, and how much weight
-- they moved between them. They are one query and not two because they must
-- share a WHERE clause exactly — a second query is a second place for that
-- filter to drift, and a total that counts sessions the list never returns is
-- worse than no total. Hence also the EXISTS guard, which is the same "at least
-- one logged rep" definition of a started session as ListSessions' HAVING.
--
-- The LEFT JOIN fans one row out per set, so the count must be DISTINCT; the
-- sum wants exactly that fan-out. volume_lb counts logged reps whether or not
-- the set was completed, matching ListSessions above.
-- name: SessionTotals :one
SELECT COUNT(DISTINCT s.id)                                     AS total,
       COALESCE(SUM(ss.actual_reps * ss.weight_lb), 0)::numeric AS volume_lb
FROM sessions s
JOIN program_days pd ON pd.id = s.program_day_id
LEFT JOIN session_sets ss ON ss.session_id = s.id
WHERE s.user_id = sqlc.arg('user_id')::int
  AND (sqlc.narg('program_id')::bigint IS NULL OR pd.program_id = sqlc.narg('program_id'))
  AND EXISTS (
    SELECT 1 FROM session_sets ls
    WHERE ls.session_id = s.id AND ls.actual_reps > 0
  );

-- UpdateSession patches metadata; NULL args leave a column unchanged.
-- name: UpdateSession :one
UPDATE sessions
SET performed_on = COALESCE(sqlc.narg('performed_on'), performed_on),
    notes        = COALESCE(sqlc.narg('notes'), notes)
WHERE id = sqlc.arg('id')
  AND user_id = sqlc.arg('user_id')::int
RETURNING id, program_day_id, performed_on, notes, created_at, finished_at, user_id;

-- FinishSession stamps the explicit end of a session. COALESCE makes it
-- idempotent: finishing an already-finished session keeps the original time
-- rather than sliding it forward on a double-tap.
-- name: FinishSession :one
UPDATE sessions
SET finished_at = COALESCE(finished_at, now())
WHERE id = sqlc.arg('id')
  AND user_id = sqlc.arg('user_id')::int
RETURNING id, program_day_id, performed_on, notes, created_at, finished_at, user_id;

-- name: DeleteSession :execrows
DELETE FROM sessions
WHERE id = sqlc.arg('id')
  AND user_id = sqlc.arg('user_id')::int;

-- ListSessionSets returns a session's logged sets in prescription order
-- (by the day's exercise position, then set number), joined to exercise names.
-- name: ListSessionSets :many
SELECT ss.id,
       ss.session_id,
       ss.exercise_id,
       e.name AS exercise_name,
       ss.set_number,
       ss.target_reps,
       ss.actual_reps,
       ss.weight_lb,
       ss.completed
FROM session_sets ss
JOIN exercises e ON e.id = ss.exercise_id
JOIN sessions s ON s.id = ss.session_id
JOIN program_day_exercises pde
  ON pde.program_day_id = s.program_day_id AND pde.exercise_id = ss.exercise_id
WHERE ss.session_id = sqlc.arg('session_id')
  AND s.user_id = sqlc.arg('user_id')::int
ORDER BY pde.position, ss.set_number;

-- GetSessionSet reaches the owner through session_sets -> sessions, so a set id
-- belonging to someone else simply does not resolve.
-- name: GetSessionSet :one
SELECT ss.id,
       ss.session_id,
       ss.exercise_id,
       e.name AS exercise_name,
       ss.set_number,
       ss.target_reps,
       ss.actual_reps,
       ss.weight_lb,
       ss.completed
FROM session_sets ss
JOIN exercises e ON e.id = ss.exercise_id
JOIN sessions s ON s.id = ss.session_id
WHERE ss.id = sqlc.arg('id')
  AND s.user_id = sqlc.arg('user_id')::int;

-- UpdateSessionSet writes all three mutable columns; the handler merges the
-- PATCH body with current values first, so actual_reps can be set to NULL to
-- clear a prior entry (COALESCE could not express that). The owner check is
-- repeated here rather than inferred from the preceding GetSessionSet: an
-- UPDATE that trusts a prior read is one refactor away from trusting nothing.
-- name: UpdateSessionSet :one
UPDATE session_sets ss
SET actual_reps = sqlc.narg('actual_reps'),
    weight_lb   = sqlc.arg('weight_lb'),
    completed   = sqlc.arg('completed')
FROM sessions s
WHERE ss.id = sqlc.arg('id')
  AND s.id = ss.session_id
  AND s.user_id = sqlc.arg('user_id')::int
RETURNING ss.id, ss.session_id, ss.exercise_id, ss.set_number, ss.target_reps, ss.actual_reps, ss.weight_lb, ss.completed;

-- ListSessionExerciseWeights returns each exercise's top working weight for the
-- given sessions, ordered by the day's exercise position — used to show a
-- per-lift weight line on each history row.
-- name: ListSessionExerciseWeights :many
SELECT ss.session_id,
       e.name                      AS exercise_name,
       COUNT(ss.id)                AS set_count,
       MAX(ss.target_reps)::int    AS reps,
       MAX(ss.weight_lb)::numeric  AS weight_lb
FROM session_sets ss
JOIN exercises e ON e.id = ss.exercise_id
JOIN sessions s ON s.id = ss.session_id
JOIN program_day_exercises pde
  ON pde.program_day_id = s.program_day_id AND pde.exercise_id = ss.exercise_id
WHERE ss.session_id = ANY(@session_ids::int[])
  AND s.user_id = sqlc.arg('user_id')::int
GROUP BY ss.session_id, e.name
ORDER BY ss.session_id, MIN(pde.position);
