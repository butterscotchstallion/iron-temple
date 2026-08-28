-- What the lifter can load, and where their lifts start. See 0013_gym_setup for
-- why these are three tables of their own rather than columns on users.
--
-- Every query here is scoped to one owner. Gym setup is not as sensitive as a
-- performance, but it is still per-lifter state, and the rule this schema
-- applies everywhere else is easier to keep than to remember exceptions to.

-- GetBarWeight returns the lifter's bar, falling back to the column default for
-- an account that has never opened the setup screen. COALESCE over a LEFT JOIN
-- rather than a plain SELECT so a missing row answers the question instead of
-- returning no rows — the caller wants a bar weight, and "45" is always a
-- truthful answer to that even when nothing has been configured.
-- name: GetBarWeight :one
SELECT COALESCE(g.bar_weight_lb, 45.0)::numeric AS bar_weight_lb
FROM users u
LEFT JOIN user_gym g ON g.user_id = u.id
WHERE u.id = sqlc.arg('user_id')::int;

-- SetBarWeight creates the row or updates it. An upsert rather than an
-- INSERT-then-UPDATE because the row's absence is the normal state, not an
-- error: see 0013.
-- name: SetBarWeight :exec
INSERT INTO user_gym (user_id, bar_weight_lb)
VALUES (sqlc.arg('user_id')::int, sqlc.arg('bar_weight_lb'))
ON CONFLICT (user_id) DO UPDATE
SET bar_weight_lb = EXCLUDED.bar_weight_lb,
    updated_at    = now();

-- ListPlates returns the lifter's inventory, heaviest first — the order the
-- greedy loader wants and the order a rack is read in.
-- name: ListPlates :many
SELECT plate_lb, pairs
FROM user_plates
WHERE user_id = sqlc.arg('user_id')::int
ORDER BY plate_lb DESC;

-- DeleteAllPlates and AddPlate are the two halves of a replace. The API writes
-- the inventory whole rather than patching a denomination at a time: the client
-- edits a rack, not a row, and a partial write would leave a plate the lifter
-- deleted still loadable.
-- name: DeleteAllPlates :exec
DELETE FROM user_plates WHERE user_id = sqlc.arg('user_id')::int;

-- name: AddPlate :exec
INSERT INTO user_plates (user_id, plate_lb, pairs)
VALUES (sqlc.arg('user_id')::int, sqlc.arg('plate_lb'), sqlc.arg('pairs'))
ON CONFLICT (user_id, plate_lb) DO UPDATE
SET pairs = EXCLUDED.pairs;

-- SeedDefaultPlates gives a new account the standard set, matching what
-- 0013_gym_setup wrote for the accounts that already existed. Called from
-- register() inside the same transaction, so an account either has an inventory
-- or does not exist — which is what lets an empty inventory mean "owns no
-- plates" rather than "never configured".
-- name: SeedDefaultPlates :exec
INSERT INTO user_plates (user_id, plate_lb, pairs)
SELECT sqlc.arg('user_id')::int, v.plate_lb, v.pairs
FROM (VALUES
    (45.0, 2),
    (35.0, 2),
    (25.0, 2),
    (10.0, 2),
    ( 5.0, 2),
    ( 2.5, 2)
) AS v(plate_lb, pairs)
ON CONFLICT DO NOTHING;

-- ListBaselines returns every starting weight the lifter has overridden. Read
-- whole rather than one lift at a time because prescribe() needs the day's worth
-- at once and a program day is a handful of lifts.
-- name: ListBaselines :many
SELECT exercise_id, weight_lb
FROM user_lift_baselines
WHERE user_id = sqlc.arg('user_id')::int;

-- SetBaseline records where a lift starts for this lifter. Upsert for the same
-- reason SetBarWeight is one.
--
-- The exercise_id FK is what scopes this: a baseline for an exercise that does
-- not exist, or for someone else's custom movement, fails on the constraint
-- rather than needing a handler check — exercises.created_by_user_id is already
-- the visibility rule, and the API checks it before calling this so the failure
-- is a 404 rather than a 500.
-- name: SetBaseline :exec
INSERT INTO user_lift_baselines (user_id, exercise_id, weight_lb)
VALUES (sqlc.arg('user_id')::int, sqlc.arg('exercise_id'), sqlc.arg('weight_lb'))
ON CONFLICT (user_id, exercise_id) DO UPDATE
SET weight_lb = EXCLUDED.weight_lb;

-- name: DeleteBaseline :execrows
DELETE FROM user_lift_baselines
WHERE user_id = sqlc.arg('user_id')::int
  AND exercise_id = sqlc.arg('exercise_id');
