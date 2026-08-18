-- Exercises are the library: the seeded catalogue everyone shares, plus the
-- movements a lifter added for themselves. created_by_user_id is what separates
-- them — NULL is shared, non-NULL is owned — and every read below carries the
-- same "mine or everyone's" filter. A custom exercise belonging to someone else
-- must be indistinguishable from one that does not exist, the same rule
-- sessions.sql applies to performances.

-- ListExercises returns the library, alphabetically.
--
-- performed_only narrows it to lifts this user has actually logged, which is
-- what the Progress page wants: a wall of untrained accessories reading "No
-- sessions yet" is noise, and it costs one history request each to render. The
-- EXISTS uses actual_reps > 0, the same definition of real work as
-- ListExerciseHistory below, so a lift appears on Progress exactly when it has a
-- history to draw.
-- name: ListExercises :many
SELECT e.id,
       e.name,
       e.muscle_group,
       e.equipment,
       e.is_accessory,
       (e.created_by_user_id IS NOT NULL)::bool AS is_custom
FROM exercises e
WHERE (e.created_by_user_id IS NULL OR e.created_by_user_id = sqlc.arg('user_id')::int)
  AND (NOT sqlc.arg('performed_only')::bool
       OR EXISTS (
           SELECT 1
           FROM session_sets ss
           JOIN sessions s ON s.id = ss.session_id
           WHERE ss.exercise_id = e.id
             AND s.user_id = sqlc.arg('user_id')::int
             AND ss.actual_reps > 0))
ORDER BY e.name;

-- name: GetExercise :one
SELECT e.id,
       e.name,
       e.muscle_group,
       e.equipment,
       e.is_accessory,
       (e.created_by_user_id IS NOT NULL)::bool AS is_custom
FROM exercises e
WHERE e.id = sqlc.arg('id')
  AND (e.created_by_user_id IS NULL OR e.created_by_user_id = sqlc.arg('user_id')::int);

-- CountExerciseNameConflicts asks whether a proposed name is already taken, in
-- the only sense that matters to the caller: a shared exercise, or one of their
-- own. Case-insensitive to match the two partial unique indexes in 0009 — the
-- database would reject the insert anyway, and this turns that into a 409 with
-- something to say rather than a 500.
-- name: CountExerciseNameConflicts :one
SELECT count(*)
FROM exercises
WHERE lower(name) = lower(sqlc.arg('name'))
  AND (created_by_user_id IS NULL OR created_by_user_id = sqlc.arg('user_id')::int);

-- name: CreateExercise :one
INSERT INTO exercises (name, muscle_group, equipment, created_by_user_id)
VALUES (sqlc.arg('name'), sqlc.arg('muscle_group'), sqlc.arg('equipment'), sqlc.arg('user_id')::int)
RETURNING id, name, muscle_group, equipment, is_accessory,
          (created_by_user_id IS NOT NULL)::bool AS is_custom;

-- CountExerciseUses reports what would break if an exercise were deleted.
--
-- logged_sets is the one that must never be overridden: session_sets.exercise_id
-- has no ON DELETE clause, so the delete would fail anyway, and a finished
-- session is a record — losing the name of a lift out of it is losing history.
-- assistance_entries would cascade cleanly, but silently rewriting someone's
-- program from the library screen is a surprise, so the API refuses that too and
-- says which day to remove it from.
-- name: CountExerciseUses :one
SELECT (SELECT count(*) FROM session_sets ss
        WHERE ss.exercise_id = sqlc.arg('exercise_id'))::bigint AS logged_sets,
       (SELECT count(*) FROM program_day_assistance pda
        WHERE pda.exercise_id = sqlc.arg('exercise_id'))::bigint AS assistance_entries;

-- DeleteExercise removes one of the caller's own movements. The
-- created_by_user_id predicate is doing two jobs: it scopes to the owner, and it
-- makes the seeded catalogue undeletable by anyone, since those rows have NULL
-- there and NULL = anything is never true.
-- name: DeleteExercise :execrows
DELETE FROM exercises
WHERE id = sqlc.arg('id')
  AND created_by_user_id = sqlc.arg('user_id')::int;

-- ListExerciseHistory returns one point per performed session for a lift
-- (oldest first): the top weight worked, the best reps, and whether every set
-- hit target. Only logged sessions (a set with actual_reps > 0) are included.
-- name: ListExerciseHistory :many
SELECT s.performed_on,
       MAX(ss.weight_lb)::numeric               AS weight_lb,
       COALESCE(MAX(ss.actual_reps), 0)::int        AS reps,
       COALESCE(BOOL_AND(ss.completed), false)::bool AS completed
FROM session_sets ss
JOIN sessions s ON s.id = ss.session_id
WHERE ss.exercise_id = sqlc.arg('exercise_id')
  AND s.user_id = sqlc.arg('user_id')::int
  AND ss.actual_reps > 0
GROUP BY s.id, s.performed_on
ORDER BY s.performed_on, s.id;
