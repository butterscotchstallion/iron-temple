-- name: ListPrograms :many
SELECT id, name, description, progression_kind
FROM programs
ORDER BY id;

-- name: GetProgram :one
SELECT id, name, description, progression_kind
FROM programs
WHERE id = $1;

-- name: GetProgramDay :one
SELECT id, program_id, name, position
FROM program_days
WHERE id = $1;

-- name: ListProgramDays :many
SELECT id, program_id, name, position
FROM program_days
WHERE program_id = $1
ORDER BY position;

-- ListPrescriptionsByProgram returns every prescribed exercise across all of a
-- program's days, joined to the exercise name, ordered for assembly in Go.
-- name: ListPrescriptionsByProgram :many
SELECT pde.id,
       pde.program_day_id,
       pde.exercise_id,
       e.name AS exercise_name,
       pde.position,
       pde.sets,
       pde.reps,
       pde.starting_weight_lb
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
       pde.starting_weight_lb
FROM program_day_exercises pde
JOIN exercises e ON e.id = pde.exercise_id
WHERE pde.program_day_id = $1
ORDER BY pde.position;

-- ListLiftHistory returns one row per past session in which a lift was
-- performed within a program, oldest first, for the progression engine.
-- Scope is the whole program (all days) because a lift such as the squat
-- progresses continuously across Workout A and B. A session "succeeds" for the
-- lift only if every logged set for it was completed; weight_lb is the top
-- weight worked that session.
-- name: ListLiftHistory :many
SELECT s.performed_on,
       MAX(ss.weight_lb)::numeric  AS weight_lb,
       BOOL_AND(ss.completed)      AS success
FROM sessions s
JOIN session_sets ss ON ss.session_id = s.id
JOIN program_days pd ON pd.id = s.program_day_id
WHERE pd.program_id = $1
  AND ss.exercise_id = $2
GROUP BY s.id, s.performed_on
ORDER BY s.performed_on, s.id;
