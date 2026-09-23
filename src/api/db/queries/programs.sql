-- Programs, and who is allowed to see which.
--
-- Since 0029 a program is either the install's (created_by_user_id IS NULL:
-- seeded, canonical, nobody's to edit) or one lifter's own. The reads below
-- split into two kinds and it is worth being clear which is which, because they
-- do NOT use the same rule:
--
--   DISCOVERY  — ListPrograms, the picker. Seeded, mine, or shared. This is the
--                one is_shared governs.
--   ACCESS     — GetProgram and CanReadProgram, reached with an id in hand.
--                Seeded, mine, shared, OR I have trained it.
--
-- That last clause is grandfathering, and it is the difference between the two.
-- Un-sharing a program means "stop new people finding it"; it cannot sensibly
-- mean "lock out the lifter who has been running it for three months", whose
-- sessions are bound to those program_day rows for ever regardless. Taking
-- their access away costs them their training and buys the owner nothing.
--
-- Archival is the same distinction one turn further: an archived program leaves
-- the picker and keeps resolving, so nobody is stranded mid-program and
-- users.current_program_id can keep pointing at it.

-- ListPrograms is the picker: what this lifter may choose from.
--
-- include_archived is the owner's own view. It never widens past created_by =
-- caller, so one lifter asking for archived programs cannot surface another's —
-- an archived shared program is retired, and retiring it is the owner saying
-- "stop offering this to people".
-- name: ListPrograms :many
SELECT p.id,
       p.name,
       p.description,
       p.progression_kind,
       p.created_by_user_id,
       p.is_shared,
       p.archived_at,
       COALESCE(u.display_name, '') AS owner_name
FROM programs p
LEFT JOIN users u ON u.id = p.created_by_user_id
WHERE (p.created_by_user_id IS NULL
       OR p.created_by_user_id = sqlc.arg('user_id')::int
       OR p.is_shared)
  AND (p.archived_at IS NULL
       OR (sqlc.arg('include_archived')::bool
           AND p.created_by_user_id = sqlc.arg('user_id')::int))
ORDER BY p.id;

-- ListSeededPrograms is the install's own catalogue, for callers that need a
-- program without a lifter to scope it to.
--
-- Exists for ensureGeneratedLifters, which assigns each generated account a
-- current_program_id by walking this list. It cannot use ListPrograms: that now
-- takes a viewer, and there is no honest one to pass — handing it the admin's id
-- would have fake lifters training a real lifter's private programs, and handing
-- it the generated account's own id returns only the seeded rows anyway, by a
-- coincidence that would break the day generated accounts could own programs.
-- name: ListSeededPrograms :many
SELECT id, name, description, progression_kind
FROM programs
WHERE created_by_user_id IS NULL
ORDER BY id;

-- GetProgram resolves one program for a caller entitled to see it, and returns
-- no rows otherwise — so a handler's existing pgx.ErrNoRows branch turns a
-- program belonging to somebody else into a 404 with no new code.
--
-- 404 and not 403 is the rule assistance.go states and sessions.sql applies:
-- learning that an id is valid is already a leak.
-- name: GetProgram :one
SELECT p.id,
       p.name,
       p.description,
       p.progression_kind,
       p.created_by_user_id,
       p.is_shared,
       p.archived_at,
       COALESCE(u.display_name, '') AS owner_name
FROM programs p
LEFT JOIN users u ON u.id = p.created_by_user_id
WHERE p.id = sqlc.arg('id')
  AND (p.created_by_user_id IS NULL
       OR p.created_by_user_id = sqlc.arg('user_id')::int
       OR p.is_shared
       OR EXISTS (
           SELECT 1
           FROM sessions s
           JOIN program_days pd ON pd.id = s.program_day_id
           WHERE pd.program_id = p.id
             AND s.user_id = sqlc.arg('user_id')::int
       ));

-- CanReadProgram is GetProgram's predicate on its own, for the paths that hold a
-- day rather than a program and only need the yes/no.
--
-- One query rather than the same EXISTS smeared through five SELECTs: the rule
-- is subtle enough that having it written once is worth a round trip. It rides
-- sessions_user_idx, so the expensive-looking clause is not.
-- name: CanReadProgram :one
SELECT EXISTS (
    SELECT 1
    FROM programs p
    WHERE p.id = sqlc.arg('program_id')
      AND (p.created_by_user_id IS NULL
           OR p.created_by_user_id = sqlc.arg('user_id')::int
           OR p.is_shared
           OR EXISTS (
               SELECT 1
               FROM sessions s
               JOIN program_days pd ON pd.id = s.program_day_id
               WHERE pd.program_id = p.id
                 AND s.user_id = sqlc.arg('user_id')::int
           ))
)::bool;

-- GetProgramDay is unscoped on purpose: it answers "what day is this", and every
-- caller resolves the program's readability separately (programDay() and
-- createSession both call CanReadProgram with day.ProgramID). Folding the check
-- in here would mean joining programs and threading a user through the two
-- callers that legitimately have no viewer — the activity generator among them.
-- name: GetProgramDay :one
SELECT id, program_id, name, position, weekday, archived_at
FROM program_days
WHERE id = $1;

-- ListProgramDays returns a program's live days. Archived ones are gone from
-- every read in the app: they exist so that a day somebody has trained can leave
-- the program without taking the session with it, not so that they can be
-- listed.
--
-- archived_at is selected even though this query filters it to NULL, so that
-- both day reads return the same row shape and the handlers that pass a day
-- around need one type rather than two identical ones.
-- name: ListProgramDays :many
SELECT id, program_id, name, position, weekday, archived_at
FROM program_days
WHERE program_id = $1
  AND archived_at IS NULL
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
  AND pd.archived_at IS NULL
ORDER BY pd.position, pde.position;

-- ListPrescriptionsByDay returns the prescribed exercises for a single day.
--
-- This is the one the progression engine reads, which is why equipment is here
-- and not on ListPrescriptionsByProgram above: what a lift can jump by is a
-- fact about the bar or the bells it uses (progression.LadderFor), and the
-- program listing prescribes nothing and needs no ladder.
-- name: ListPrescriptionsByDay :many
SELECT pde.id,
       pde.program_day_id,
       pde.exercise_id,
       e.name AS exercise_name,
       pde.position,
       pde.sets,
       pde.reps,
       pde.starting_weight_lb,
       e.rest_seconds,
       e.equipment
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
  AND pd.archived_at IS NULL
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
