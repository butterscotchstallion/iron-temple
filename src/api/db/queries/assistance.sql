-- Assistance work: the exercises a lifter bolts onto the end of a program day.
--
-- This table is the reason a SEEDED program never has to be edited. The seeded
-- program_days and program_day_exercises are shared, and stay exactly as they
-- were seeded; assistance is a per-user overlay keyed on (user_id,
-- program_day_id). So the same Workout A is squat/bench/row for every account,
-- the progression engine reads a prescription nobody has touched, and what one
-- lifter adds is invisible to the next.
--
-- Since 0029 a lifter can also build a program of their own, whose prescription
-- they may edit directly. The overlay does not become redundant: it is still how
-- you add work to a seeded program, still how you add work to somebody else's
-- shared one, and still the only kind of addition that is private to you.
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
-- same reason 0011 put it on exercises in the first place. equipment comes with
-- it and for the same reason: double progression moves the weight by the
-- smallest jump the rack allows, and only the exercise knows whether that is a
-- bar's 5 lb or a pair of bells' 10.
-- name: ListAssistanceByDay :many
SELECT pda.id,
       pda.program_day_id,
       pda.exercise_id,
       e.name AS exercise_name,
       pda.position,
       pda.sets,
       pda.reps,
       pda.weight_lb,
       pda.weight_set_after_session_id,
       pda.rep_min,
       pda.rep_max,
       e.rest_seconds,
       e.equipment
FROM program_day_assistance pda
JOIN exercises e ON e.id = pda.exercise_id
WHERE pda.program_day_id = sqlc.arg('program_day_id')
  AND pda.user_id = sqlc.arg('user_id')::int
ORDER BY pda.position, pda.id;

-- ListAssistanceByProgram returns every day's assistance across one program, so
-- the program detail response can be assembled in one round trip. Ordered by day
-- then position, mirroring ListPrescriptionsByProgram.
--
-- equipment rides along for the same reason it does on the by-day query, but for
-- the client rather than the engine: the program page lets a lifter edit a
-- prescription, and the number input's step and the copy promising "+N lb next
-- time" both have to name the jump this movement can actually make.
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
       pda.rep_max,
       e.equipment
FROM program_day_assistance pda
JOIN exercises e ON e.id = pda.exercise_id
JOIN program_days pd ON pd.id = pda.program_day_id
WHERE pd.program_id = sqlc.arg('program_id')
  AND pd.archived_at IS NULL
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
--
-- session_id rides along so the caller can tell whether 0022's weight pin has
-- been spent. The pin records which session was last at the moment a lifter set
-- the weight by hand; it outranks the carry-forward exactly while that is still
-- the latest session, so the comparison needs the id this query already ranked
-- by and would otherwise throw away.
-- name: LastAssistanceSets :many
SELECT actual_reps, weight_lb, session_id
FROM (
    SELECT ss.actual_reps,
           ss.weight_lb,
           ss.set_number,
           s.id AS session_id,
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
       pda.rep_max,
       e.equipment
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
--
-- Naming a weight arms the pin 0022 added, which is what makes the edit outrank
-- the carry-forward until the lift is next performed.
--
-- The trigger is the field being PRESENT in the patch, decided by the handler
-- and passed in, rather than the new weight differing from the stored one.
-- Comparing against the stored weight looks equivalent and is not: the stored
-- weight is what the lift was ADDED at, while the number the lifter is looking
-- at is the prescribed one the carry-forward has since moved. A curl added at
-- 30 that has climbed to 50 is stored as 30, so "set it to 30" compares equal
-- and would arm nothing — which is precisely the bug this column exists to fix,
-- surviving inside its own fix. Presence is the only signal that means "a
-- lifter typed this", and PATCH already carries it.
--
-- The subquery is the same "which session was this lift last done in" that
-- LastAssistanceSets ranks by, down to the actual_reps > 0 filter — a set
-- nobody touched is not a session the lift was performed in, and the two have
-- to agree or a pin could be armed against a session the prescription does not
-- consider the latest. COALESCE to 0 for a lift never logged, so "no session
-- yet" is a value that can match rather than a NULL that never does.
-- name: UpdateAssistance :one
UPDATE program_day_assistance pda
SET sets      = sqlc.arg('sets'),
    reps      = sqlc.arg('reps'),
    weight_lb = sqlc.arg('weight_lb'),
    rep_min   = sqlc.narg('rep_min'),
    rep_max   = sqlc.narg('rep_max'),
    weight_set_after_session_id = CASE
        WHEN NOT sqlc.arg('pin_weight')::boolean
            THEN pda.weight_set_after_session_id
        ELSE COALESCE((
            SELECT s.id
            FROM session_sets ss
            JOIN sessions s ON s.id = ss.session_id
            WHERE ss.exercise_id = pda.exercise_id
              AND s.user_id = pda.user_id
              AND ss.actual_reps > 0
            ORDER BY s.performed_on DESC, s.id DESC
            LIMIT 1
        ), 0)
    END
WHERE pda.id = sqlc.arg('id')
  AND pda.user_id = sqlc.arg('user_id')::int
RETURNING id, program_day_id, exercise_id, position, sets, reps, weight_lb, rep_min, rep_max;

-- name: DeleteAssistance :execrows
DELETE FROM program_day_assistance
WHERE id = sqlc.arg('id')
  AND user_id = sqlc.arg('user_id')::int;
