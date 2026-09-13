-- The session recap: one workout in review, the moment it ends.
--
-- Everything here feeds internal/racked's BuildSession, which is the same
-- reducer the monthly recap uses with a slice of one. That is deliberate and it
-- is what these queries exist to serve: a record set on a Tuesday evening and
-- the same record listed in March's recap are one claim, decided once, and a
-- second implementation of "is this a PR" would be a second opinion.
--
-- Scoping matches sessions.sql and racked.sql exactly: s.user_id filters every
-- query. The "real logged work" rule (actual_reps > 0) holds everywhere except
-- RecapSessionSets, which has a reason of its own — see there.
--
-- Four of these cut the history at "before this session". They all spell that
-- cut the same way, and it is NOT `performed_on < $performed_on`:
--
--     (s.performed_on < $performed_on
--       OR (s.performed_on = $performed_on AND s.id < $session_id))
--
-- A plain date cut silently swallows an earlier session performed on the SAME
-- day, and two sessions in one day is not an exotic case — a lifter who squats
-- in the morning and presses in the evening has two. Swallowed, the morning's
-- work vanishes from the evening's baseline, so the evening is credited the
-- same personal record and the same "first 225 lb Squat" milestone all over
-- again. The pair ordering matches the (performed_on DESC, id DESC) that
-- LastWeighIn and ListSessions already sort by, so "before" means the same
-- thing here as it does there.

-- RecapSessionSets returns every set of one session — including the ones the
-- lifter never logged.
--
-- This is the single place that departs from the actual_reps > 0 rule the rest
-- of the recap queries keep, and the departure is the point. The recap reports
-- reps hit against reps prescribed, and a workout abandoned after two sets of
-- five is only legible as "10 of 25 reps" if the other fifteen are still in the
-- result. internal/racked splits the rows on arrival: those with reps become the
-- Sets that statistics are computed from, and all of them together are the
-- prescription that those statistics are reported against. Nothing downstream
-- counts an unlogged row as work.
--
-- Ordering, is_assistance and the two LEFT joins are lifted from ListSessionSets
-- unchanged, including why they are LEFT — a finished session is a record, and a
-- prescription edited afterwards must not make a set disappear from it.
-- name: RecapSessionSets :many
SELECT ss.id,
       ss.exercise_id,
       e.name AS exercise_name,
       e.muscle_group,
       ss.set_number,
       ss.target_reps,
       ss.actual_reps,
       ss.weight_lb,
       ss.completed,
       (pde.id IS NULL)::bool AS is_assistance
FROM session_sets ss
JOIN exercises e ON e.id = ss.exercise_id
JOIN sessions s ON s.id = ss.session_id
LEFT JOIN program_day_exercises pde
  ON pde.program_day_id = s.program_day_id AND pde.exercise_id = ss.exercise_id
LEFT JOIN program_day_assistance pda
  ON pda.program_day_id = s.program_day_id
 AND pda.exercise_id = ss.exercise_id
 AND pda.user_id = s.user_id
WHERE ss.session_id = sqlc.arg('session_id')
  AND s.user_id = sqlc.arg('user_id')::int
ORDER BY COALESCE(pde.position, 1000 + pda.position, 2000), ss.exercise_id, ss.set_number;

-- RecapPreviousDaySessions returns the earlier sessions of this same program
-- day, newest first — the history that "how fast was that?" is answered against.
--
-- Only sessions with real logged work, via the HAVING clause, matching
-- ListLiftHistory: a session opened by mistake and abandoned is not a previous
-- performance of the day and must not dilute a median.
--
-- created_at and finished_at come back raw rather than as a computed interval so
-- that the 12-hour cap lives in exactly one place — session.Duration() in Go —
-- rather than being restated in SQL where it could drift from it. An unfinished
-- session comes back with a NULL finished_at and is dropped there, which is also
-- why this does not filter on finished_at: whether a session counts toward pace
-- is a judgement, and judgements are made in internal/racked.
--
-- Unbounded on purpose. One day of one program is roughly fifty rows a year.
-- name: RecapPreviousDaySessions :many
SELECT s.id,
       s.performed_on,
       s.created_at,
       s.finished_at
FROM sessions s
JOIN session_sets ss ON ss.session_id = s.id
WHERE s.user_id = sqlc.arg('user_id')::int
  AND s.program_day_id = sqlc.arg('program_day_id')::int
  AND (s.performed_on < sqlc.arg('performed_on')::date
    OR (s.performed_on = sqlc.arg('performed_on')::date
        AND s.id < sqlc.arg('session_id')::int))
GROUP BY s.id, s.performed_on, s.created_at, s.finished_at
HAVING COUNT(ss.id) FILTER (WHERE ss.actual_reps > 0) > 0
ORDER BY s.performed_on DESC, s.id DESC;

-- RecapPreviousDaySets returns the logged sets of the single most recent earlier
-- session of this program day — the workout every "vs. last time" figure in the
-- recap is measured against.
--
-- The subselect repeats RecapPreviousDaySessions' filter rather than taking an
-- id from it, so the two cannot disagree about which session "last time" was;
-- the alternative is a second round trip whose answer has to be trusted to match.
--
-- Only logged sets here, unlike RecapSessionSets: the previous session is a
-- reference for what was lifted, and what it failed to lift is not this recap's
-- story to tell.
-- name: RecapPreviousDaySets :many
SELECT ss.exercise_id,
       e.name AS exercise_name,
       ss.actual_reps,
       ss.weight_lb
FROM session_sets ss
JOIN exercises e ON e.id = ss.exercise_id
WHERE ss.actual_reps > 0
  AND ss.session_id = (
    SELECT s.id
    FROM sessions s
    JOIN session_sets s2 ON s2.session_id = s.id
    WHERE s.user_id = sqlc.arg('user_id')::int
      AND s.program_day_id = sqlc.arg('program_day_id')::int
      AND (s.performed_on < sqlc.arg('performed_on')::date
        OR (s.performed_on = sqlc.arg('performed_on')::date
            AND s.id < sqlc.arg('session_id')::int))
    GROUP BY s.id, s.performed_on
    HAVING COUNT(s2.id) FILTER (WHERE s2.actual_reps > 0) > 0
    ORDER BY s.performed_on DESC, s.id DESC
    LIMIT 1
  )
ORDER BY ss.exercise_id, ss.set_number;

-- RecapExerciseBaseline is each lift's all-time best BEFORE this session, which
-- is what lets a set inside it be recognised as a record rather than merely as
-- the heaviest thing in one workout.
--
-- It is RackedExerciseBaseline with the period cut replaced by the session cut
-- described at the top of this file. The ROUND and the single-rep CASE are
-- copied verbatim and must stay that way: Set.E1RM in Go rounds to the pound and
-- compares straight against this number, so if only one side rounded, the two
-- would disagree inside a sub-pound band — and that band is exactly where a
-- record is decided. Unrounded here, repeating an identical set reads as a new
-- record, because 185 x 3 is 203.5 and Go calls it 204.
-- name: RecapExerciseBaseline :many
SELECT ss.exercise_id,
       MAX(ss.weight_lb)::numeric AS best_weight_lb,
       MAX(ROUND(CASE WHEN ss.actual_reps = 1
                      THEN ss.weight_lb
                      ELSE ss.weight_lb * (1 + ss.actual_reps / 30.0)
                 END))::numeric AS best_e1rm_lb
FROM session_sets ss
JOIN sessions s ON s.id = ss.session_id
WHERE s.user_id = sqlc.arg('user_id')::int
  AND (s.performed_on < sqlc.arg('performed_on')::date
    OR (s.performed_on = sqlc.arg('performed_on')::date
        AND s.id < sqlc.arg('session_id')::int))
  AND ss.actual_reps > 0
GROUP BY ss.exercise_id;

-- RecapVolumeBefore is the lifetime tonnage moved before this session, which is
-- what dates a volume milestone to the workout that crossed it. Same formula as
-- SessionTotals and RackedVolumeBefore, so the three cannot disagree about what
-- a pound is, and the same session cut as the baseline above.
-- name: RecapVolumeBefore :one
SELECT COALESCE(SUM(ss.actual_reps * ss.weight_lb), 0)::numeric AS volume_lb
FROM session_sets ss
JOIN sessions s ON s.id = ss.session_id
WHERE s.user_id = sqlc.arg('user_id')::int
  AND (s.performed_on < sqlc.arg('performed_on')::date
    OR (s.performed_on = sqlc.arg('performed_on')::date
        AND s.id < sqlc.arg('session_id')::int))
  AND ss.actual_reps > 0;

-- RecapSessionOutcomes returns this session and the ones before it, newest
-- first, reduced to the two numbers a streak is judged on: how many sets the
-- session carried and how many were completed. Both streaks the recap reports —
-- the run of perfect sessions and the run of weeks trained — are walked from
-- this one list in Go.
--
-- Every program day, not just this one. A streak is about showing up, and
-- alternating Workout A and B is showing up twice.
--
-- "Completed" is COUNT(*) FILTER (WHERE ss.completed), against COUNT(ss.id) for
-- the whole session, which is the same pair isSessionComplete uses in the UI
-- (setCount vs completedSetCount). The HAVING clause drops sessions with nothing
-- logged so an accidental tap cannot break a run.
--
-- LIMIT 120 is about a year of training at three sessions a week. A streak
-- longer than the window is reported as the window, which is a ceiling nobody
-- will reach honestly and a bound on a query that would otherwise grow forever.
-- name: RecapSessionOutcomes :many
SELECT s.id,
       s.performed_on,
       COUNT(ss.id)::int AS set_count,
       COUNT(*) FILTER (WHERE ss.completed)::int AS completed_set_count
FROM sessions s
JOIN session_sets ss ON ss.session_id = s.id
WHERE s.user_id = sqlc.arg('user_id')::int
  AND (s.performed_on < sqlc.arg('performed_on')::date
    OR (s.performed_on = sqlc.arg('performed_on')::date
        AND s.id <= sqlc.arg('session_id')::int))
GROUP BY s.id, s.performed_on
HAVING COUNT(ss.id) FILTER (WHERE ss.actual_reps > 0) > 0
ORDER BY s.performed_on DESC, s.id DESC
LIMIT 120;
