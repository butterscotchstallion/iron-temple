-- Racked is the monthly/yearly recap: a headline volume, a few hero moments and
-- a set of charts. Almost none of that is expressed here.
--
-- The statistics themselves live in internal/racked as pure functions over the
-- rows below, rather than as SQL. Two reasons. The first is testability: a
-- deload, a comeback and an archetype are judgements about a series, and a
-- judgement that can only be exercised against a live Postgres is one that will
-- not be exercised often. The second is that sqlc cannot be run in the
-- development sandbox, so every query here has to be hand-carried into
-- internal/store and kept byte-compatible with the generator — a cost paid per
-- query, and paid again on every edit. Three plain queries is a surface worth
-- maintaining; the dozen window functions the full stat list would otherwise
-- need is not.
--
-- A period's row count is small enough that the trade costs nothing: three
-- sessions a week of five sets across three lifts is roughly 2,300 rows a year,
-- for one user, read once when the page loads or the recap sends.
--
-- Scoping matches sessions.sql exactly and is not optional: s.user_id filters
-- every query, and actual_reps > 0 is the same "real logged work" definition
-- that ListSessions' HAVING clause uses. A recap that counted prescribed sets
-- nobody performed would flatter the lifter, which is the one thing it must
-- not do.

-- RackedPeriodSets returns every logged set performed in [start_on, end_on],
-- oldest first, with the session context each statistic needs: the date for
-- streaks and weekday buckets, created_at/finished_at for session pace, and the
-- exercise for per-lift series.
--
-- Sessions are not filtered on is_over here, matching ListSessions: work that
-- was logged counts as work whether or not the lifter tapped Finish. The pace
-- statistics do care, and apply their own guard in Go — an unfinished session
-- has a NULL finished_at and simply does not have a duration to report.
--
-- The ORDER BY is load-bearing. internal/racked walks these rows in one pass and
-- relies on sets arriving grouped by session in chronological order, so PRs are
-- detected against the state of the history at that moment rather than at the
-- end of it.
-- name: RackedPeriodSets :many
SELECT s.id AS session_id,
       s.performed_on,
       s.created_at,
       s.finished_at,
       s.program_day_id,
       pd.name AS program_day_name,
       ss.exercise_id,
       e.name  AS exercise_name,
       ss.set_number,
       ss.actual_reps,
       ss.weight_lb,
       ss.completed
FROM session_sets ss
JOIN sessions s ON s.id = ss.session_id
JOIN program_days pd ON pd.id = s.program_day_id
JOIN exercises e ON e.id = ss.exercise_id
WHERE s.user_id = sqlc.arg('user_id')::int
  AND s.performed_on >= sqlc.arg('start_on')
  AND s.performed_on <= sqlc.arg('end_on')
  AND ss.actual_reps > 0
ORDER BY s.performed_on, s.id, ss.exercise_id, ss.set_number;

-- RackedExerciseBaseline returns each lift's all-time best before the period
-- opened. It is what makes a personal record inside the period a record rather
-- than merely a maximum: without it, the first session of a lifter's second year
-- would set a "PR" on every lift they own.
--
-- Both bests are carried because they answer different questions. best_weight_lb
-- is the plate-milestone number a lifter recognises; best_e1rm_lb is the Epley
-- estimate that a heavier single and a longer set can be compared on. The Go
-- side keeps the same two, and estimateOneRepMax in the UI uses the same
-- formula — weight x (1 + reps/30).
--
-- ROUND is load-bearing, not cosmetic. Set.E1RM in Go rounds to the pound, and
-- personalRecords compares an in-period estimate straight against this baseline;
-- if only one side rounded, the two would disagree inside a sub-pound band and
-- that band is exactly where a record is decided. Unrounded here, repeating an
-- identical set in a later period reads as a new record — 185 x 3 is 203.5, the
-- Go side calls it 204, and 204 > 203.5. Rounding per row rather than around the
-- MAX mirrors what Go does, and is equivalent anyway since ROUND is monotonic.
-- name: RackedExerciseBaseline :many
SELECT ss.exercise_id,
       MAX(ss.weight_lb)::numeric AS best_weight_lb,
       MAX(ROUND(ss.weight_lb * (1 + ss.actual_reps / 30.0)))::numeric AS best_e1rm_lb
FROM session_sets ss
JOIN sessions s ON s.id = ss.session_id
WHERE s.user_id = sqlc.arg('user_id')::int
  AND s.performed_on < sqlc.arg('start_on')
  AND ss.actual_reps > 0
GROUP BY ss.exercise_id;

-- RackedVolumeBefore is the lifetime tonnage moved before the period opened,
-- which turns a volume milestone into an event with a date: crossing a million
-- pounds is only news in the month it happens. Same formula as SessionTotals so
-- the two can never disagree about what a pound is.
-- name: RackedVolumeBefore :one
SELECT COALESCE(SUM(ss.actual_reps * ss.weight_lb), 0)::numeric AS volume_lb
FROM session_sets ss
JOIN sessions s ON s.id = ss.session_id
WHERE s.user_id = sqlc.arg('user_id')::int
  AND s.performed_on < sqlc.arg('start_on')
  AND ss.actual_reps > 0;
