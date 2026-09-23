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

-- SetDumbbellStep records what this lifter's rack steps by, per bell. Upsert
-- for the same reason SetBarWeight is one; a row created here takes the column
-- default for the bar, which is the number GetBarWeight would have answered
-- anyway while the row was missing.
-- name: SetDumbbellStep :exec
INSERT INTO user_gym (user_id, dumbbell_step_lb)
VALUES (sqlc.arg('user_id')::int, sqlc.arg('dumbbell_step_lb'))
ON CONFLICT (user_id) DO UPDATE
SET dumbbell_step_lb = EXCLUDED.dumbbell_step_lb,
    updated_at       = now();

-- GetGymSteps answers the one question the progression engine asks of the gym:
-- what is the smallest weight change this lifter can actually make? One read
-- rather than two because both halves are wanted together, by prescribe(), for
-- every lift on the day.
--
-- The bar's step is derived, not configured. Two of the lightest plate owned is
-- the finest change a barbell admits, and the inventory is already recorded —
-- asking the lifter to also state the consequence of their own rack would be a
-- second fact that can disagree with the first. Owning no plates falls back to
-- 5 rather than 0: an empty inventory means bar-only, which admits no change at
-- all, and a step of zero is not a number the engine can divide by.
--
-- The rack's step is per BELL, as 0020 stores it. Doubling it is left to Go,
-- beside the comment explaining why the pair is what gets prescribed.
--
-- The machine, cable and band steps are stored rather than derived, because
-- unlike the bar there is nothing to derive them FROM: this app records no
-- inventory of pins or of graded bands, only what the lifter says the gaps are.
-- Before 0028 they were not stored either, and every one of them silently took
-- the bar's — see the argument in that migration.
--
-- equipment_confirmed_at is deliberately NOT here. This row is what the
-- progression engine reads, and whether a lifter has reviewed their equipment
-- must never change what gets prescribed from it (0028 again). Keeping it off
-- the struct the engine consumes is a stronger guarantee of that than a comment
-- would be; the profile reads it separately, below.
-- name: GetGymSteps :one
SELECT
    COALESCE(
        (SELECT MIN(p.plate_lb) * 2 FROM user_plates p WHERE p.user_id = u.id),
        5.0
    )::numeric AS bar_step_lb,
    COALESCE(g.dumbbell_step_lb, 5.0)::numeric AS dumbbell_step_lb,
    COALESCE(g.machine_step_lb, 5.0)::numeric AS machine_step_lb,
    COALESCE(g.cable_step_lb, 5.0)::numeric AS cable_step_lb,
    COALESCE(g.band_step_lb, 5.0)::numeric AS band_step_lb
FROM users u
LEFT JOIN user_gym g ON g.user_id = u.id
WHERE u.id = sqlc.arg('user_id')::int;

-- SetEquipmentSteps records the three stacks in one write. Upsert for the same
-- reason SetBarWeight is one: the row's absence is the normal state.
--
-- All three together rather than one query each, because the screen that
-- collects them saves them together — and because a partial write would leave a
-- gym half-described by the lifter and half-assumed by us, which is precisely
-- the state 0028 exists to make visible.
-- name: SetEquipmentSteps :exec
INSERT INTO user_gym (user_id, machine_step_lb, cable_step_lb, band_step_lb)
VALUES (
    sqlc.arg('user_id')::int,
    sqlc.arg('machine_step_lb'),
    sqlc.arg('cable_step_lb'),
    sqlc.arg('band_step_lb')
)
ON CONFLICT (user_id) DO UPDATE
SET machine_step_lb = EXCLUDED.machine_step_lb,
    cable_step_lb   = EXCLUDED.cable_step_lb,
    band_step_lb    = EXCLUDED.band_step_lb,
    updated_at      = now();

-- GetEquipmentConfirmed answers whether this lifter has ever reviewed the gym
-- the app assembled for them, for the copy that asks them to. NULL means no.
--
-- Its own query rather than a field on GetGymSteps, so the flag cannot reach
-- the progression engine by accident. See the note there.
-- name: GetEquipmentConfirmed :one
SELECT g.equipment_confirmed_at
FROM users u
LEFT JOIN user_gym g ON g.user_id = u.id
WHERE u.id = sqlc.arg('user_id')::int;

-- ConfirmEquipment marks the gym as described by its owner rather than guessed
-- by us. Called whenever a lifter saves the equipment screen — saving IS the
-- confirmation, so there is no separate button to forget to press.
--
-- Idempotent by design: re-saving moves the timestamp forward, which is the
-- more useful reading anyway ("last told us on…") and spares the caller a
-- check it would otherwise have to make.
-- name: ConfirmEquipment :exec
INSERT INTO user_gym (user_id, equipment_confirmed_at)
VALUES (sqlc.arg('user_id')::int, now())
ON CONFLICT (user_id) DO UPDATE
SET equipment_confirmed_at = now(),
    updated_at             = now();

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
