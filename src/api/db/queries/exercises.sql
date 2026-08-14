-- name: ListExercises :many
SELECT id, name
FROM exercises
ORDER BY name;

-- name: GetExercise :one
SELECT id, name
FROM exercises
WHERE id = $1;

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
