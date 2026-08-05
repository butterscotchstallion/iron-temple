-- name: ListExercises :many
SELECT id, name
FROM exercises
ORDER BY name;

-- name: GetExercise :one
SELECT id, name
FROM exercises
WHERE id = $1;
