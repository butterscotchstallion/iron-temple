-- name: ListPrograms :many
SELECT id, name, description, progression_kind
FROM programs
ORDER BY id;

-- name: GetProgram :one
SELECT id, name, description, progression_kind
FROM programs
WHERE id = $1;

-- name: GetProgramDay :one
SELECT id, program_id, name, position, weekday
FROM program_days
WHERE id = $1;

-- name: ListProgramDays :many
SELECT id, program_id, name, position, weekday
FROM program_days
WHERE program_id = $1
ORDER BY position;

-- name: UpdateProgramDayWeekday :one
UPDATE program_days
SET weekday = sqlc.narg('weekday')
WHERE id = sqlc.arg('id')
RETURNING id, program_id, name, position, weekday;

-- ListPrescriptionsByProgram returns every prescribed exercise across all of a
-- program's days, joined to the exercise name, ordered for assembly in Go.
--
-- rest_seconds comes off the exercise rather than the prescription: rest is a
-- property of the movement, so a squat rests the same on Workout A and B. See
-- 0011 for the tiers.
-- name: ListPrescriptionsByProgram :many
SELECT pde.id,
       pde.program_day_id,
       pde.exercise_id,
       e.name AS exercise_name,
       pde.position,
       pde.sets,
       pde.reps,
       pde.starting_weight_lb,
       e.rest_seconds
FROM program_day_exercises pde
JOIN program_days pd ON pd.id = pde.program_day_id
JOIN exercises e ON e.id = pde.exercise_id
WHERE pd.program_id = $1
ORDER BY pd.position, pde.position;

-- ListPrescriptionsByDay returns the prescribed exercises for a single day.
-- name: ListPrescriptionsByDay :many
SELECT pde.id,
       pde.program_day_id,
       pde.exercise_id,
       e.name AS exercise_name,
       pde.position,
       pde.sets,
       pde.reps,
       pde.starting_weight_lb,
       e.rest_seconds
FROM program_day_exercises pde
JOIN exercises e ON e.id = pde.exercise_id
WHERE pde.program_day_id = $1
ORDER BY pde.position;

-- ListLiftHistory returns one row per past session in which a lift was
-- performed, oldest first, for the progression engine. A session "succeeds" for
-- the lift only if every logged set for it was completed; weight_lb is the top
-- weight worked that session.
--
-- Scope is the lift and the lifter, deliberately NOT the program. A squat is a
-- squat: the bar does not know which program day sent you to it, and neither
-- should the engine. Scoping this to one program used to mean that switching
-- programs restarted every lift at its seeded starting weight — which fell
-- hardest on exactly the move the app recommends, since Advanced 3x5 is the
-- graduation fork when 5x5 stalls and taking it dropped a working squat back to
-- an empty bar. ListExerciseHistory has always read across programs for the
-- same reason (assistance.sql calls it "dips are dips whichever day they were
-- done on"); this is the main lifts agreeing with it.
--
-- The consequence to keep in mind: a lift's history is now one series, so a
-- deload or a stall follows you between programs as well. That is the intent —
-- a stall is a fact about the lifter, not about the program they were running
-- when it happened.
--
-- Only sessions that are over and carry real logged work count. Sets are
-- materialized up front with completed = false, so without both guards a
-- session you merely started — or are still in the middle of — would score as
-- BOOL_AND(completed) = false and be recorded as a failed session, pushing the
-- engine toward an unearned deload. See the is_over note in sessions.sql.
--
-- The user_id filter matters more here than anywhere else: this feeds the
-- progression engine, so an unscoped history would compute one lifter's next
-- working weight from another's performance — a wrong number on the bar, not
-- merely a privacy leak.
-- name: ListLiftHistory :many
SELECT s.performed_on,
       MAX(ss.weight_lb)::numeric  AS weight_lb,
       BOOL_AND(ss.completed)      AS success
FROM sessions s
JOIN session_sets ss ON ss.session_id = s.id
WHERE ss.exercise_id = sqlc.arg('exercise_id')
  AND s.user_id = sqlc.arg('user_id')::int
  AND (s.finished_at IS NOT NULL
       OR s.created_at < now() - INTERVAL '12 hours')
GROUP BY s.id, s.performed_on
HAVING COUNT(ss.id) FILTER (WHERE ss.actual_reps > 0) > 0
ORDER BY s.performed_on, s.id;

-- ListSetPlansByProgram returns every per-set prescription in a program, so a
-- day's ramps and its lifts' reference days can both be resolved without a query
-- per lift.
--
-- Scoped to the program rather than to the day because the two questions have
-- different scopes: what to load TODAY needs this day's rungs, but which day is
-- a lift's reference needs every day's. Reading the program once answers both.
--
-- Empty for every program but Madcow. An absent set plan means a uniform block
-- of sets x reps at one weight, which is what the other five prescribe.
-- name: ListSetPlansByProgram :many
SELECT pde.program_day_id,
       pde.exercise_id,
       s.set_number,
       s.reps,
       s.pct_of_top
FROM program_day_exercise_sets s
JOIN program_day_exercises pde ON pde.id = s.program_day_exercise_id
JOIN program_days pd ON pd.id = pde.program_day_id
WHERE pd.program_id = sqlc.arg('program_id')
ORDER BY pde.program_day_id, pde.exercise_id, s.set_number;

-- ListLiftHistoryForDay is ListLiftHistory narrowed to one program day, for the
-- Madcow engine's top set.
--
-- The narrowing is the whole point and is not an optimisation. A lift's top set
-- is decided on its reference day alone: the squat's ramp tops at 87.5% on the
-- light day and 102.5% on the intensity day, so a history taking every day's
-- heaviest set would see the number wander and read it as progress and regress
-- that never happened.
--
-- Same is_over and actual_reps guards as ListLiftHistory, for the same reasons —
-- see the note there, which this query is otherwise a copy of.
-- name: ListLiftHistoryForDay :many
SELECT s.performed_on,
       MAX(ss.weight_lb)::numeric  AS weight_lb,
       BOOL_AND(ss.completed)      AS success
FROM sessions s
JOIN session_sets ss ON ss.session_id = s.id
WHERE ss.exercise_id = sqlc.arg('exercise_id')
  AND s.user_id = sqlc.arg('user_id')::int
  AND s.program_day_id = sqlc.arg('program_day_id')
  AND (s.finished_at IS NOT NULL
       OR s.created_at < now() - INTERVAL '12 hours')
GROUP BY s.id, s.performed_on
HAVING COUNT(ss.id) FILTER (WHERE ss.actual_reps > 0) > 0
ORDER BY s.performed_on, s.id;
