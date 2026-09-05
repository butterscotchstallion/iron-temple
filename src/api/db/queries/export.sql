-- ExportAssistance returns every accessory this lifter has attached to a program
-- day, named rather than keyed.
--
-- The whole file is written for one caller: GET /me/export, which hands the
-- lifter their entire account as a single JSON document. Two properties follow
-- from that and shape every query here.
--
-- Names, not ids. An export is read by a human, a spreadsheet, or an importer
-- into a database whose SERIAL sequences are somewhere else entirely; a column
-- reading `exercise_id: 47` is worth nothing to any of them. Ids that survive
-- into the output are there only to stitch the document together (a set names
-- the session it belongs to), never as the record of what a thing is.
--
-- A fixed number of queries, whatever the history's size. The obvious shape —
-- list the sessions, then fetch each one's sets — is a round trip per session,
-- which for a few years of training is a few hundred. Sets are read for the
-- whole account in one pass and grouped in Go instead.
-- name: ExportAssistance :many
SELECT p.name  AS program_name,
       pd.name AS program_day_name,
       e.name  AS exercise_name,
       pda.sets,
       pda.reps,
       pda.rep_min,
       pda.rep_max,
       pda.weight_lb,
       pda.created_at
FROM program_day_assistance pda
JOIN program_days pd ON pd.id = pda.program_day_id
JOIN programs p ON p.id = pd.program_id
JOIN exercises e ON e.id = pda.exercise_id
WHERE pda.user_id = sqlc.arg('user_id')::int
ORDER BY p.name, pd.position, pda.position, pda.id;

-- ExportBaselines returns the lifter's starting-weight overrides with the lift
-- named.
--
-- ListBaselines already reads this table, and is deliberately not reused: it
-- returns exercise_id alone because its caller (prescribe) is holding the
-- exercise rows anyway, and an id is exactly what an export must not carry.
-- name: ExportBaselines :many
SELECT e.name AS exercise_name,
       ulb.weight_lb
FROM user_lift_baselines ulb
JOIN exercises e ON e.id = ulb.exercise_id
WHERE ulb.user_id = sqlc.arg('user_id')::int
ORDER BY e.name;

-- ExportCustomExercises returns the movements this lifter added to the library.
--
-- Scoped to created_by_user_id rather than the whole library on purpose. The
-- seeded catalogue ships with the app and is identical in every install, so
-- exporting it would pad the document with a hundred rows the lifter did not
-- write and an importer already has. What is theirs is what they created.
-- name: ExportCustomExercises :many
SELECT e.name,
       e.muscle_group,
       e.equipment,
       e.is_accessory,
       e.rest_seconds,
       e.created_at
FROM exercises e
WHERE e.created_by_user_id = sqlc.arg('user_id')::int
ORDER BY e.name;

-- ExportSessionSets returns every set this lifter has ever logged, across every
-- session, in one pass.
--
-- session_id is here to group by, and is the one id in the export that means
-- anything: it matches the id ExportSessions returns, which is what lets the
-- handler nest sets under their session without a query each.
--
-- is_assistance is derived by the same LEFT JOIN ListSessionSets uses, and for
-- the same reason — whether a set was on the program's own list is a question
-- the join can still answer for a lift the lifter has since removed. INNER would
-- make the ordering a filter and quietly drop those sets from the export, which
-- is the one thing an export may never do.
-- name: ExportSessionSets :many
SELECT ss.session_id,
       e.name AS exercise_name,
       ss.set_number,
       ss.target_reps,
       ss.actual_reps,
       ss.weight_lb,
       ss.completed,
       (pde.id IS NULL)::bool AS is_assistance
FROM session_sets ss
JOIN sessions s ON s.id = ss.session_id
JOIN exercises e ON e.id = ss.exercise_id
LEFT JOIN program_day_exercises pde
  ON pde.program_day_id = s.program_day_id AND pde.exercise_id = ss.exercise_id
WHERE s.user_id = sqlc.arg('user_id')::int
ORDER BY ss.session_id, ss.exercise_id, ss.set_number;

-- ExportSessions returns every session this lifter has, oldest first.
--
-- Unpaged, and without the "at least one logged rep" HAVING that ListSessions
-- applies. Both of those are display rules: the history screen shows sessions
-- worth reading, a page at a time. An export is the record, so a session that
-- was started and abandoned is part of it — the lifter can drop those rows
-- themselves, and cannot recover ones this query decided not to return.
-- name: ExportSessions :many
SELECT s.id,
       p.name  AS program_name,
       pd.name AS program_day_name,
       s.performed_on,
       s.notes,
       s.bodyweight_lb,
       s.created_at,
       s.finished_at
FROM sessions s
JOIN program_days pd ON pd.id = s.program_day_id
JOIN programs p ON p.id = pd.program_id
WHERE s.user_id = sqlc.arg('user_id')::int
ORDER BY s.performed_on, s.id;
