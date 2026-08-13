-- A session is "over" when the lifter finished it by hand, or when it was
-- started more than 12 hours ago and so cannot still be in progress. This is
-- derived at read time rather than materialized: no background job has to run,
-- and there is no window in which a stale row disagrees with the clock. The
-- expression is repeated in the queries below because a generated column cannot
-- use now(), which Postgres does not consider immutable. Keep the copies in
-- sync — GetSession, ListSessions and ListLiftHistory all depend on it.
--
--     s.finished_at IS NOT NULL OR s.created_at < now() - INTERVAL '12 hours'

-- name: CreateSession :one
INSERT INTO sessions (program_day_id, performed_on)
VALUES ($1, $2)
RETURNING id, program_day_id, performed_on, notes, created_at, finished_at;

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
WHERE s.id = $1;

-- ListSessions returns paginated session summaries, most recent first,
-- optionally filtered to one program. Pass NULL program_id for all programs.
-- is_over reads s.finished_at/s.created_at under a GROUP BY: legal because the
-- grouping includes s.id, the primary key, which makes every other sessions
-- column functionally dependent on it.
-- name: ListSessions :many
SELECT s.id,
       s.program_day_id,
       pd.name AS program_day_name,
       p.id    AS program_id,
       p.name  AS program_name,
       s.performed_on,
       COUNT(ss.id)                              AS set_count,
       COUNT(ss.id) FILTER (WHERE ss.completed)  AS completed_set_count,
       (s.finished_at IS NOT NULL
        OR s.created_at < now() - INTERVAL '12 hours')::bool AS is_over
FROM sessions s
JOIN program_days pd ON pd.id = s.program_day_id
JOIN programs p ON p.id = pd.program_id
LEFT JOIN session_sets ss ON ss.session_id = s.id
WHERE (sqlc.narg('program_id')::bigint IS NULL OR p.id = sqlc.narg('program_id'))
GROUP BY s.id, pd.name, p.id, p.name
-- Only sessions with at least one logged rep count as "started".
HAVING COUNT(ss.id) FILTER (WHERE ss.actual_reps > 0) > 0
ORDER BY s.performed_on DESC, s.id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountSessions :one
SELECT COUNT(*) AS total
FROM sessions s
JOIN program_days pd ON pd.id = s.program_day_id
WHERE (sqlc.narg('program_id')::bigint IS NULL OR pd.program_id = sqlc.narg('program_id'))
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
RETURNING id, program_day_id, performed_on, notes, created_at, finished_at;

-- FinishSession stamps the explicit end of a session. COALESCE makes it
-- idempotent: finishing an already-finished session keeps the original time
-- rather than sliding it forward on a double-tap.
-- name: FinishSession :one
UPDATE sessions
SET finished_at = COALESCE(finished_at, now())
WHERE id = $1
RETURNING id, program_day_id, performed_on, notes, created_at, finished_at;

-- name: DeleteSession :execrows
DELETE FROM sessions WHERE id = $1;

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
WHERE ss.session_id = $1
ORDER BY pde.position, ss.set_number;

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
WHERE ss.id = $1;

-- UpdateSessionSet writes all three mutable columns; the handler merges the
-- PATCH body with current values first, so actual_reps can be set to NULL to
-- clear a prior entry (COALESCE could not express that).
-- name: UpdateSessionSet :one
UPDATE session_sets
SET actual_reps = sqlc.narg('actual_reps'),
    weight_lb   = sqlc.arg('weight_lb'),
    completed   = sqlc.arg('completed')
WHERE id = sqlc.arg('id')
RETURNING id, session_id, exercise_id, set_number, target_reps, actual_reps, weight_lb, completed;

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
GROUP BY ss.session_id, e.name
ORDER BY ss.session_id, MIN(pde.position);
