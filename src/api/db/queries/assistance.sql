-- Assistance work: the exercises a lifter bolts onto the end of a program day.
--
-- This table is the reason programs never have to be edited. program_days and
-- program_day_exercises are shared and seeded, and stay that way; assistance is
-- a per-user overlay keyed on (user_id, program_day_id). So the same Workout A
-- is squat/bench/row for every account, the progression engine reads a
-- prescription nobody has touched, and what one lifter adds is invisible to the
-- next.
--
-- Every query is scoped to one owner. That is not defence in depth here, it is
-- the whole isolation model: program days are shared, so an unscoped read would
-- hand one lifter another's plan on a row they are equally entitled to see.
--
-- Deleting a row deletes a plan, never a performance. Sets already logged
-- against the exercise stay in session_sets and keep counting toward volume and
-- records — which is why ListSessionSets orders with a fallback for sets whose
-- assistance row has since gone.

-- ListAssistanceByDay returns one day's assistance in display order.
--
-- rest_seconds rides along from the exercise so a prescribed session can carry a
-- rest for assistance work too, without this table needing a column for it — the
-- same reason 0011 put it on exercises in the first place.
-- name: ListAssistanceByDay :many
SELECT pda.id,
       pda.program_day_id,
       pda.exercise_id,
       e.name AS exercise_name,
       pda.position,
       pda.sets,
       pda.reps,
       pda.weight_lb,
       pda.rep_min,
       pda.rep_max,
       e.rest_seconds
FROM program_day_assistance pda
JOIN exercises e ON e.id = pda.exercise_id
WHERE pda.program_day_id = sqlc.arg('program_day_id')
  AND pda.user_id = sqlc.arg('user_id')::int
ORDER BY pda.position, pda.id;

-- ListAssistanceByProgram returns every day's assistance across one program, so
-- the program detail response can be assembled in one round trip. Ordered by day
-- then position, mirroring ListPrescriptionsByProgram.
-- name: ListAssistanceByProgram :many
SELECT pda.id,
       pda.program_day_id,
       pda.exercise_id,
       e.name AS exercise_name,
       pda.position,
       pda.sets,
       pda.reps,
       pda.weight_lb,
       pda.rep_min,
       pda.rep_max
FROM program_day_assistance pda
JOIN exercises e ON e.id = pda.exercise_id
JOIN program_days pd ON pd.id = pda.program_day_id
WHERE pd.program_id = sqlc.arg('program_id')
  AND pda.user_id = sqlc.arg('user_id')::int
ORDER BY pd.position, pda.position, pda.id;

-- LastAssistanceSets returns the reps actually logged on each set of a lift, the
-- last time it was performed, with the weight worked.
--
-- The double-progression engine needs per-set reps, which is what makes this its
-- own query rather than an extension of ListExerciseHistory — that returns one
-- row per session with the top weight and the best reps, a shape three other
-- callers depend on and which cannot answer "did EVERY set reach the top of the
-- range".
--
-- actual_reps > 0 keeps unlogged sets out of the answer: a set nobody touched is
-- not a set that fell short, and counting it would stop the range from ever
-- topping out. Deliberately not scoped to a finished session, matching the
-- carry-forward weight this sits beside — a lifter mid-workout is looking at
-- what they just did.
--
-- DENSE_RANK rather than a correlated subquery so the "which session was last"
-- decision is made once, in the same pass that reads the rows.
-- name: LastAssistanceSets :many
SELECT actual_reps, weight_lb
FROM (
    SELECT ss.actual_reps,
           ss.weight_lb,
           ss.set_number,
           DENSE_RANK() OVER (ORDER BY s.performed_on DESC, s.id DESC) AS recency
    FROM session_sets ss
    JOIN sessions s ON s.id = ss.session_id
    WHERE ss.exercise_id = sqlc.arg('exercise_id')
      AND s.user_id = sqlc.arg('user_id')::int
      AND ss.actual_reps > 0
) ranked
WHERE recency = 1
ORDER BY set_number;

-- name: GetAssistance :one
SELECT pda.id,
       pda.program_day_id,
       pda.exercise_id,
       e.name AS exercise_name,
       pda.position,
       pda.sets,
       pda.reps,
       pda.weight_lb,
       pda.rep_min,
       pda.rep_max
FROM program_day_assistance pda
JOIN exercises e ON e.id = pda.exercise_id
WHERE pda.id = sqlc.arg('id')
  AND pda.user_id = sqlc.arg('user_id')::int;

-- CreateAssistance appends to the end of the day's assistance block. position is
-- computed in the INSERT rather than read and incremented by the handler: two
-- concurrent adds that both read "3" would both write 4, and ordering by
-- (position, id) would then be settled by insertion order anyway — but the
-- subquery keeps the numbers honest for nothing extra.
-- name: CreateAssistance :one
INSERT INTO program_day_assistance
    (user_id, program_day_id, exercise_id, position, sets, reps, weight_lb, rep_min, rep_max)
VALUES (
    sqlc.arg('user_id')::int,
    sqlc.arg('program_day_id'),
    sqlc.arg('exercise_id'),
    (SELECT COALESCE(MAX(position), 0) + 1
     FROM program_day_assistance
     WHERE user_id = sqlc.arg('user_id')::int
       AND program_day_id = sqlc.arg('program_day_id')),
    sqlc.arg('sets'),
    sqlc.arg('reps'),
    sqlc.arg('weight_lb'),
    sqlc.narg('rep_min'),
    sqlc.narg('rep_max')
)
RETURNING id, program_day_id, exercise_id, position, sets, reps, weight_lb, rep_min, rep_max;

-- UpdateAssistance writes every mutable column; the handler merges the PATCH
-- body with current values first, the same shape as UpdateSessionSet. The owner
-- check is repeated here rather than inferred from the preceding GetAssistance,
-- for the reason given there.
--
-- rep_min and rep_max are narg rather than arg because NULL is meaningful: it is
-- how a lifter turns the rep range back off and returns the lift to carrying its
-- weight forward. A COALESCE here would make that unsayable.
-- name: UpdateAssistance :one
UPDATE program_day_assistance
SET sets      = sqlc.arg('sets'),
    reps      = sqlc.arg('reps'),
    weight_lb = sqlc.arg('weight_lb'),
    rep_min   = sqlc.narg('rep_min'),
    rep_max   = sqlc.narg('rep_max')
WHERE id = sqlc.arg('id')
  AND user_id = sqlc.arg('user_id')::int
RETURNING id, program_day_id, exercise_id, position, sets, reps, weight_lb, rep_min, rep_max;

-- name: DeleteAssistance :execrows
DELETE FROM program_day_assistance
WHERE id = sqlc.arg('id')
  AND user_id = sqlc.arg('user_id')::int;
