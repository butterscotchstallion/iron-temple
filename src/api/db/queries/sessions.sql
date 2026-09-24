-- A session is "over" when the lifter finished it by hand, or when it was
-- started more than 12 hours ago and so cannot still be in progress. This is
-- derived at read time rather than materialized: no background job has to run,
-- and there is no window in which a stale row disagrees with the clock. The
-- expression is repeated in the queries below because a generated column cannot
-- use now(), which Postgres does not consider immutable. Keep the copies in
-- sync — GetSession, ListSessions and ListLiftHistory all depend on it.
--
--     s.finished_at IS NOT NULL OR s.created_at < now() - INTERVAL '12 hours'
--
-- Every query here is scoped to one owner (s.user_id = user_id). That filter is
-- the whole of the isolation model, so it is not optional on any read or write:
-- programs and exercises are shared, but performances are not. A session the
-- caller does not own must be indistinguishable from one that does not exist,
-- which is why the set-level queries below reach the owner through a JOIN back
-- to sessions rather than trusting the set id alone — a caller learning that
-- someone else's set id is valid is already a leak.
--
-- The three RETURNING lists on sessions name every column of the table, in the
-- table's own order, and that is deliberate: it is what makes sqlc reuse the
-- Session model struct instead of emitting a bespoke row type per query. A
-- column added to sessions has to be added to all three, or the next generation
-- silently splits them apart.

-- name: CreateSession :one
INSERT INTO sessions (program_day_id, performed_on, user_id)
VALUES (sqlc.arg('program_day_id'), sqlc.arg('performed_on'), sqlc.arg('user_id')::int)
RETURNING id, program_day_id, performed_on, notes, created_at, finished_at, user_id, bodyweight_lb;

-- name: CreateSessionSet :one
INSERT INTO session_sets (session_id, exercise_id, set_number, target_reps, weight_lb)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, session_id, exercise_id, set_number, target_reps, actual_reps, weight_lb, completed;

-- name: GetSession :one
SELECT s.id,
       s.program_day_id,
       pd.name AS program_day_name,
       p.id    AS program_id,
       p.name  AS program_name,
       s.performed_on,
       s.notes,
       s.created_at,
       s.finished_at,
       s.bodyweight_lb,
       (s.finished_at IS NOT NULL
        OR s.created_at < now() - INTERVAL '12 hours')::bool AS is_over
FROM sessions s
JOIN program_days pd ON pd.id = s.program_day_id
JOIN programs p ON p.id = pd.program_id
WHERE s.id = sqlc.arg('id')
  AND s.user_id = sqlc.arg('user_id')::int;

-- LastWeighIn returns the lifter's most recent recorded bodyweight, which the
-- session screen pre-fills its box with so a new session needs a nudge rather
-- than a fresh entry.
--
-- Deliberately excludes the session being read. A session's own weigh-in is
-- already on it (GetSession above); what this answers is "what did you last
-- weigh BEFORE this one", and a session that carried itself would report a
-- number the lifter had already entered as something waiting to be entered.
--
-- "Most recent" is by performed_on, not by created_at, so a back-dated session
-- lands where the lifter says it happened. Note that this is the latest weigh-in
-- overall rather than the latest one before this session's date: a session
-- back-dated into the middle of a history pre-fills with today's weight, not the
-- weight of the week it is filed under. That is the right default for the only
-- caller — the box is a scale reading taken now — and the wrong one for a chart,
-- which should read the column directly rather than through this query.
--
-- :one, so no prior weigh-in is pgx.ErrNoRows and not an empty row. The caller
-- has to translate that into "none" rather than let it escape as a 404.
-- name: LastWeighIn :one
SELECT s.performed_on,
       s.bodyweight_lb
FROM sessions s
WHERE s.user_id = sqlc.arg('user_id')::int
  AND s.id <> sqlc.arg('exclude_session_id')
  AND s.bodyweight_lb IS NOT NULL
ORDER BY s.performed_on DESC, s.id DESC
LIMIT 1;

-- ListSessions returns paginated session summaries, most recent first,
-- optionally filtered to one program. Pass NULL program_id for all programs.
-- is_over reads s.finished_at/s.created_at under a GROUP BY: legal because the
-- grouping includes s.id, the primary key, which makes every other sessions
-- column functionally dependent on it.
--
-- volume_lb is the weight actually moved: actual_reps, not target_reps, and
-- every logged set rather than only the completed ones — a set that stopped at
-- 3 of 5 reps still moved the bar three times. SUM already skips the NULL
-- actual_reps of an unlogged set; COALESCE covers a session where none is
-- logged. The ::numeric cast is what types the column for sqlc.
-- name: ListSessions :many
SELECT s.id,
       s.program_day_id,
       pd.name AS program_day_name,
       p.id    AS program_id,
       p.name  AS program_name,
       s.performed_on,
       COUNT(ss.id)                              AS set_count,
       COUNT(ss.id) FILTER (WHERE ss.completed)  AS completed_set_count,
       COALESCE(SUM(ss.actual_reps * ss.weight_lb), 0)::numeric AS volume_lb,
       (s.finished_at IS NOT NULL
        OR s.created_at < now() - INTERVAL '12 hours')::bool AS is_over
FROM sessions s
JOIN program_days pd ON pd.id = s.program_day_id
JOIN programs p ON p.id = pd.program_id
LEFT JOIN session_sets ss ON ss.session_id = s.id
WHERE s.user_id = sqlc.arg('user_id')::int
  AND (sqlc.narg('program_id')::bigint IS NULL OR p.id = sqlc.narg('program_id'))
GROUP BY s.id, pd.name, p.id, p.name
-- Only sessions with at least one logged rep count as "started".
HAVING COUNT(ss.id) FILTER (WHERE ss.actual_reps > 0) > 0
ORDER BY s.performed_on DESC, s.id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- ListLifterSessions is ListSessions pointed at somebody else.
--
-- Two user ids, and confusing them is the bug this query exists to avoid.
-- lifter_id is WHOSE sessions these are and scopes the rows; viewer_id is who is
-- READING and decides only whether a program's name is legible to them. The
-- second one can never widen the first — it appears nowhere in the WHERE.
--
-- Everything else is ListSessions verbatim: the same columns, the same volume_lb
-- definition, the same is_over expression, the same HAVING and the same order. A
-- lifter's history read by somebody else must not count a session their own
-- history does not, or value one differently, because the two screens link to
-- the same recap.
--
-- No program_id filter, unlike ListSessions. Filtering another lifter's history
-- by program is not a question any surface asks, and the narg would be a
-- parameter every caller passes NULL for.
--
-- The masking CASE is ListFeedSessions', inline for the same reason and kept in
-- step with maskedProgramName in internal/api. This is the second read in the
-- app that hands a lifter a fact about a program they have no access to, and it
-- leaks by exactly the same route: a session on a private program, listed to
-- somebody who was never shown it. pd.name is deliberately NOT masked — see the
-- longer note on ListFeedSessions for why a day name is a far weaker signal.
-- name: ListLifterSessions :many
SELECT s.id,
       s.program_day_id,
       pd.name AS program_day_name,
       p.id    AS program_id,
       CASE WHEN p.created_by_user_id IS NULL
                 OR p.is_shared
                 OR p.created_by_user_id = sqlc.arg('viewer_id')::int
                 OR EXISTS (SELECT 1 FROM sessions vs
                            JOIN program_days vpd ON vpd.id = vs.program_day_id
                            WHERE vpd.program_id = p.id
                              AND vs.user_id = sqlc.arg('viewer_id')::int)
            THEN p.name
            ELSE 'Custom program'
       END AS program_name,
       s.performed_on,
       COUNT(ss.id)                              AS set_count,
       COUNT(ss.id) FILTER (WHERE ss.completed)  AS completed_set_count,
       COALESCE(SUM(ss.actual_reps * ss.weight_lb), 0)::numeric AS volume_lb,
       (s.finished_at IS NOT NULL
        OR s.created_at < now() - INTERVAL '12 hours')::bool AS is_over
FROM sessions s
JOIN program_days pd ON pd.id = s.program_day_id
JOIN programs p ON p.id = pd.program_id
LEFT JOIN session_sets ss ON ss.session_id = s.id
WHERE s.user_id = sqlc.arg('lifter_id')::int
GROUP BY s.id, pd.name, p.id, p.name, p.created_by_user_id, p.is_shared
-- Only sessions with at least one logged rep count as "started".
HAVING COUNT(ss.id) FILTER (WHERE ss.actual_reps > 0) > 0
ORDER BY s.performed_on DESC, s.id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- SessionTotals returns the two figures that describe a whole history rather
-- than one page of it: how many sessions match the filter, and how much weight
-- they moved between them. They are one query and not two because they must
-- share a WHERE clause exactly — a second query is a second place for that
-- filter to drift, and a total that counts sessions the list never returns is
-- worse than no total. Hence also the EXISTS guard, which is the same "at least
-- one logged rep" definition of a started session as ListSessions' HAVING.
--
-- The LEFT JOIN fans one row out per set, so the count must be DISTINCT; the
-- sum wants exactly that fan-out. volume_lb counts logged reps whether or not
-- the set was completed, matching ListSessions above.
-- name: SessionTotals :one
SELECT COUNT(DISTINCT s.id)                                     AS total,
       COALESCE(SUM(ss.actual_reps * ss.weight_lb), 0)::numeric AS volume_lb
FROM sessions s
JOIN program_days pd ON pd.id = s.program_day_id
LEFT JOIN session_sets ss ON ss.session_id = s.id
WHERE s.user_id = sqlc.arg('user_id')::int
  AND (sqlc.narg('program_id')::bigint IS NULL OR pd.program_id = sqlc.narg('program_id'))
  AND EXISTS (
    SELECT 1 FROM session_sets ls
    WHERE ls.session_id = s.id AND ls.actual_reps > 0
  );

-- ListFeedSessions is what the OTHER lifters on this install have been doing,
-- most recent first.
--
-- The one query in this file that is not scoped to a single user, and the
-- exclusion is the scoping: viewer_id is the caller, and their own sessions are
-- left out rather than included. That is not a filter a client could as easily
-- apply itself.
--
-- Two reasons it belongs here. A lifter's own history already has a page — this
-- would duplicate it, and on the install this app was built for (exactly one
-- lifter) a feed of everybody would be that page a second time under a new name.
-- And a client-side filter cannot page: asking for ten and discarding the six
-- that are yours leaves four, so the surface either shows a short page it cannot
-- explain or pages again to fill it. Excluded in SQL, ten means ten.
--
-- The happy consequence is that a single-lifter install gets an empty feed, so
-- the surfaces that draw it stand down on their own and that install keeps the
-- shape it always had. Nothing has to special-case "is anybody else here".
--
-- Everything else mirrors ListSessions deliberately — the same columns, the same
-- volume_lb definition (actual_reps, every logged set rather than only the
-- completed ones), the same is_over expression, and the same HAVING. A feed that
-- counted a session the history page does not, or valued one differently, would
-- be the app disagreeing with itself about the row a lifter can open and read.
--
-- Offset paging, also matching ListSessions. A feed is the one place keyset
-- paging would genuinely be better — rows arrive at the top while somebody
-- pages — but consistency with the endpoint this is a sibling of is worth more
-- than correctness against a race that, on an install with a handful of lifters
-- paging a list nobody is racing them through, does not happen.
--
-- No total, unlike ListSessions. "How many sessions have the others ever logged"
-- is not a question this surface asks, and it would cost a second aggregate to
-- answer: a caller pages until a short page tells them to stop.
--
-- JOIN users, not LEFT: a session with user_id IS NULL predates accounts existing
-- and was adopted by the first registration (see AdoptOrphanSessions). An
-- unadopted one is owned by nobody, so it belongs in no lifter's feed, and the
-- inner join drops it without needing a predicate that says so.
--
-- avatar_etag is joined for the reason ListLifters joins it: the feed draws an
-- avatar per row, and looking one up per row would be a query per entry. Bytes
-- are never read, only the tag.
--
-- The LEFT JOIN on session_sets fans a session out per set, which is what the
-- aggregates want; ua and the users join cannot fan, being keyed on the PK. Both
-- ua.etag and the program columns are listed in GROUP BY rather than left to
-- functional dependency: that inference works through the grouped table's own
-- primary key, and Postgres will not carry it across a join even where user_id
-- happens to be both sides' key.
-- name: ListFeedSessions :many
SELECT s.id,
       s.user_id,
       u.username,
       u.display_name,
       u.avatar_color,
       COALESCE(ua.etag, '') AS avatar_etag,
       s.program_day_id,
       pd.name AS program_day_name,
       p.id    AS program_id,
       -- The program's name, unless it is one this viewer was never shown.
       --
       -- This is the one read in the app that hands a lifter a fact about a
       -- program they have no access to. The feed is other people's sessions by
       -- definition, so without this the moment somebody logs a session on a
       -- private program its name reaches the whole install — a leak invisible
       -- from inside the program code, since nothing there touches the feed.
       --
       -- The predicate is CanReadProgram's, inline rather than called: it is
       -- evaluated per row over a page of at most a few dozen, and factoring it
       -- out would mean a lateral join for one boolean.
       --
       -- pd.name is deliberately NOT masked. A feed card is about the day — "Ada
       -- finished Workout A" — and "Custom program · " with nothing after it is a
       -- card not worth drawing. A day name is a far weaker signal anyway: it
       -- says what somebody trained, which is the point of a feed, rather than
       -- what plan they are following.
       CASE WHEN p.created_by_user_id IS NULL
                 OR p.is_shared
                 OR p.created_by_user_id = sqlc.arg('viewer_id')::int
                 OR EXISTS (SELECT 1 FROM sessions vs
                            JOIN program_days vpd ON vpd.id = vs.program_day_id
                            WHERE vpd.program_id = p.id
                              AND vs.user_id = sqlc.arg('viewer_id')::int)
            THEN p.name
            ELSE 'Custom program'
       END AS program_name,
       s.performed_on,
       COUNT(ss.id)                              AS set_count,
       COUNT(ss.id) FILTER (WHERE ss.completed)  AS completed_set_count,
       COALESCE(SUM(ss.actual_reps * ss.weight_lb), 0)::numeric AS volume_lb,
       (s.finished_at IS NOT NULL
        OR s.created_at < now() - INTERVAL '12 hours')::bool AS is_over,
       -- How much applause and conversation the session drew, so a feed row can
       -- show that something landed well without the feed itself becoming
       -- interactive: tapping a reaction needs the session in front of you, which
       -- is the recap the row links to.
       --
       -- Scalar subqueries rather than two more LEFT JOINs. The joins would
       -- multiply against each other and against session_sets — a session with 15
       -- sets, 3 reactions and 2 comments would fan to 90 rows — and every
       -- aggregate above would then need DISTINCT to survive it. These are
       -- counted independently, which is what they are.
       (SELECT COUNT(*) FROM session_reactions sr WHERE sr.session_id = s.id)::bigint
         AS reaction_count,
       (SELECT COUNT(*) FROM session_comments sc WHERE sc.session_id = s.id)::bigint
         AS comment_count
FROM sessions s
JOIN users u ON u.id = s.user_id
LEFT JOIN user_avatars ua ON ua.user_id = u.id
JOIN program_days pd ON pd.id = s.program_day_id
JOIN programs p ON p.id = pd.program_id
LEFT JOIN session_sets ss ON ss.session_id = s.id
WHERE s.user_id <> sqlc.arg('viewer_id')::int
GROUP BY s.id, u.id, u.username, u.display_name, u.avatar_color, ua.etag,
         pd.name, p.id, p.name
-- Only sessions with at least one logged rep count as "started", exactly as
-- ListSessions has it.
HAVING COUNT(ss.id) FILTER (WHERE ss.actual_reps > 0) > 0
ORDER BY s.performed_on DESC, s.id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- UpdateSession patches metadata; NULL args leave a column unchanged.
--
-- bodyweight_lb cannot use that convention, because clearing a weigh-in and
-- leaving it alone are different requests and COALESCE renders them identically
-- — the same problem, and the same reasoning, as actual_reps in UpdateSessionSet
-- below. set_bodyweight is what separates them: false means the PATCH body had
-- no bodyweightLb key at all, true means it had one, and the value may then be
-- NULL to erase the entry. The ::numeric cast types the branch for sqlc, which
-- has no column to infer from inside a CASE.
-- name: UpdateSession :one
UPDATE sessions
SET performed_on  = COALESCE(sqlc.narg('performed_on'), performed_on),
    notes         = COALESCE(sqlc.narg('notes'), notes),
    bodyweight_lb = CASE WHEN sqlc.arg('set_bodyweight')::bool
                         THEN sqlc.narg('bodyweight_lb')::numeric
                         ELSE bodyweight_lb END
WHERE id = sqlc.arg('id')
  AND user_id = sqlc.arg('user_id')::int
RETURNING id, program_day_id, performed_on, notes, created_at, finished_at, user_id, bodyweight_lb;

-- FinishSession stamps the explicit end of a session. COALESCE makes it
-- idempotent: finishing an already-finished session keeps the original time
-- rather than sliding it forward on a double-tap.
-- name: FinishSession :one
UPDATE sessions
SET finished_at = COALESCE(finished_at, now())
WHERE id = sqlc.arg('id')
  AND user_id = sqlc.arg('user_id')::int
RETURNING id, program_day_id, performed_on, notes, created_at, finished_at, user_id, bodyweight_lb;

-- name: DeleteSession :execrows
DELETE FROM sessions
WHERE id = sqlc.arg('id')
  AND user_id = sqlc.arg('user_id')::int;

-- ListSessionSets returns a session's logged sets in prescription order (the
-- day's main lifts by position, then that lifter's assistance, then set number),
-- joined to exercise names.
--
-- Both position joins are LEFT, and that is load-bearing rather than defensive.
-- A set exists because it was materialized into the session; the joins here only
-- decide what order to read it in. An INNER JOIN makes ordering a filter, so any
-- set without a current row on the other side disappears from a finished session
-- — from a record that is supposed to be immutable. Three ways that happens:
-- assistance work, which has no program_day_exercises row at all; assistance the
-- lifter removed after performing it; and, before assistance existed, a
-- prescription edited out from under old sessions.
--
--     COALESCE(pde.position, 1000 + pda.position, 2000)
--
-- reads as: main lifts in prescribed order, then assistance below them, then
-- anything orphaned last. The 1000 offset is a separator, not a limit on how
-- many exercises a day may hold — positions are small integers assigned max+1,
-- and a day would need a thousand prescribed lifts to collide.
--
-- is_assistance is derived from the same join rather than stored on the set: the
-- session materializes whatever prescribe() returned, and asking "was this on
-- the program's own list?" at read time cannot drift from the answer the
-- ordering above already depends on.
--
-- equipment rides along from the exercise for the same reason ListAssistanceByDay
-- takes it: the smallest jump a lift can make, and whether it has a bar to warm
-- up with at all, are properties of the movement and nothing else knows them. The
-- session screen draws a plate diagram and an empty-bar opener off this, which on
-- a pair of dumbbells describes equipment that is not in the lifter's hands.
-- name: ListSessionSets :many
SELECT ss.id,
       ss.session_id,
       ss.exercise_id,
       e.name AS exercise_name,
       ss.set_number,
       ss.target_reps,
       ss.actual_reps,
       ss.weight_lb,
       ss.completed,
       ss.is_bonus,
       (pde.id IS NULL)::bool AS is_assistance,
       e.rest_seconds,
       e.equipment
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

-- GetSessionSet reaches the owner through session_sets -> sessions, so a set id
-- belonging to someone else simply does not resolve.
--
-- is_assistance is derived the same way as in ListSessionSets — absence of a
-- program_day_exercises row for the day — so the field a PATCH echoes back
-- cannot disagree with the one the session was read with. Only the pde join is
-- needed here; this query does no ordering, so it has no use for pda.
-- name: GetSessionSet :one
SELECT ss.id,
       ss.session_id,
       ss.exercise_id,
       e.name AS exercise_name,
       ss.set_number,
       ss.target_reps,
       ss.actual_reps,
       ss.weight_lb,
       ss.completed,
       ss.is_bonus,
       (pde.id IS NULL)::bool AS is_assistance,
       e.rest_seconds,
       e.equipment
FROM session_sets ss
JOIN exercises e ON e.id = ss.exercise_id
JOIN sessions s ON s.id = ss.session_id
LEFT JOIN program_day_exercises pde
  ON pde.program_day_id = s.program_day_id AND pde.exercise_id = ss.exercise_id
WHERE ss.id = sqlc.arg('id')
  AND s.user_id = sqlc.arg('user_id')::int;

-- AppendSessionSet adds one more set to a lift already in the session, copying
-- the rep target and weight from that lift's current last set — an extra set is
-- another set of the same thing, and anything else is what the weight stepper
-- and the rep counter are for.
--
-- set_number is MAX + 1, computed inside the INSERT rather than read and
-- incremented by the handler: two concurrent taps that both read "5" would both
-- write 6 and one would lose to session_sets' UNIQUE. Same argument
-- CreateAssistance makes for position.
--
-- Taking the last set by ORDER BY ... LIMIT 1 also means gaps are harmless: a
-- deleted set leaves its number unused, and the next append lands past it rather
-- than trying to fill it.
--
-- Returns no rows when the lift is not in the session, which is how the handler
-- tells a bad exerciseId from a real failure. Ownership and whether the session
-- is still open are checked before this runs, so they are 404 and 409 rather
-- than an indistinguishable empty result.
--
-- is_bonus is decided here too, and for the same reason set_number is: it is a
-- question about the state of the session at the instant of the insert, and a
-- handler that read it first would be answering about a slightly older session
-- than the one it writes to. The NOT EXISTS runs against the same snapshot as
-- the rest of the statement, so the answer cannot be stale by the time it is
-- stored. This is also the ONLY place it is ever written — see 0027 for why a
-- set's bonus-ness is a fact about the moment it was added and not something a
-- later read can recover.
--
-- The rule is "everything else in the session was already done". Two details of
-- how that is spelled are deliberate:
--
--   actual_reps IS NULL  — outstanding means NOT YET WORKED THROUGH, not "missed
--   its target". Those are different questions and this column answers the
--   first; `completed` answers the second. ActiveSession sets completed only
--   when the reps meet or beat the target ("Every set hit its target reps — a
--   clean session"), so a 4-of-5 grind is a set the lifter is finished with but
--   which completed still calls false. Testing NOT prior.completed here — as
--   this query did when the column shipped — therefore let ONE missed rep
--   anywhere in the session silently disqualify every later append from ever
--   being bonus work.
--
--   That was backwards in the worst way, because a miss is exactly when bonus
--   work happens. The same screen only auto-finishes a session when every set
--   hit its target ("A miss anywhere leaves it running until the lifter says
--   so"), so the session that is still open — the one with extra sets being
--   added to it — is disproportionately the session that contains a miss.
--
--   It is the whole session and not just this lift, either way: an extra set of
--   squats while the bench work is still untouched is a mid-workout adjustment,
--   not bonus work, and the lifter reads it as one.
--
--   A set the lifter skipped outright is indistinguishable from one they have
--   not reached yet — both are NULL — so both block. That is the conservative
--   reading: bailing on rows to do extra squats is a substitution, not a bonus.
--
--   NOT prior.is_bonus   — an earlier bonus set that is not yet logged does not
--   stop the next one counting. Without this, tapping "add set" twice before
--   logging either would make the first a bonus and the second not, which is an
--   arbitrary distinction between two taps of the same gesture. What makes the
--   gesture a bonus is that the PRESCRIBED work is finished; bonus sets are the
--   gesture itself and cannot disqualify each other.
-- name: AppendSessionSet :one
INSERT INTO session_sets (session_id, exercise_id, set_number, target_reps, weight_lb, is_bonus)
SELECT last.session_id,
       last.exercise_id,
       last.set_number + 1,
       last.target_reps,
       last.weight_lb,
       NOT EXISTS (
           SELECT 1
           FROM session_sets prior
           WHERE prior.session_id = last.session_id
             AND prior.actual_reps IS NULL
             AND NOT prior.is_bonus
       )
FROM (
    SELECT ss.session_id, ss.exercise_id, ss.set_number, ss.target_reps, ss.weight_lb
    FROM session_sets ss
    JOIN sessions s ON s.id = ss.session_id
    WHERE ss.session_id = sqlc.arg('session_id')
      AND ss.exercise_id = sqlc.arg('exercise_id')
      AND s.user_id = sqlc.arg('user_id')::int
    ORDER BY ss.set_number DESC
    LIMIT 1
) AS last
RETURNING id, session_id, exercise_id, set_number, target_reps, actual_reps, weight_lb, completed, is_bonus;

-- DeleteSessionSet removes one set. Reaches the owner through sessions, the same
-- way GetSessionSet does, so a set id belonging to someone else does not resolve
-- rather than reporting a different failure.
--
-- Nothing renumbers afterwards. set_number is ordering, not identity: every read
-- here sorts by it and none of them assume it is dense, and renumbering inside
-- UNIQUE (session_id, exercise_id, set_number) would need a deferred constraint
-- to avoid colliding with itself mid-update. A gap costs nothing.
--
-- Deleting a lift's last set is allowed, and takes the lift out of the session.
-- That is a real thing to want — the rows were skipped — and it reads correctly
-- everywhere downstream, since a lift with no sets simply has nothing to report.
-- name: DeleteSessionSet :execrows
DELETE FROM session_sets ss
USING sessions s
WHERE ss.id = sqlc.arg('id')
  AND s.id = ss.session_id
  AND s.user_id = sqlc.arg('user_id')::int;

-- UpdateSessionSet writes all three mutable columns; the handler merges the
-- PATCH body with current values first, so actual_reps can be set to NULL to
-- clear a prior entry (COALESCE could not express that). The owner check is
-- repeated here rather than inferred from the preceding GetSessionSet: an
-- UPDATE that trusts a prior read is one refactor away from trusting nothing.
--
-- is_bonus is returned but never assigned, and that omission is the point: it
-- records how a set came to exist, which logging it does not change. A bonus
-- set is still a bonus set once its reps are in, and a prescribed set does not
-- become one by being finished last. AppendSessionSet is its only writer.
-- name: UpdateSessionSet :one
UPDATE session_sets ss
SET actual_reps = sqlc.narg('actual_reps'),
    weight_lb   = sqlc.arg('weight_lb'),
    completed   = sqlc.arg('completed')
FROM sessions s
WHERE ss.id = sqlc.arg('id')
  AND s.id = ss.session_id
  AND s.user_id = sqlc.arg('user_id')::int
RETURNING ss.id, ss.session_id, ss.exercise_id, ss.set_number, ss.target_reps, ss.actual_reps, ss.weight_lb, ss.completed, ss.is_bonus;

-- ListSessionExerciseWeights returns each exercise's top working weight for the
-- given sessions, ordered by the day's exercise position — used to show a
-- per-lift weight line on each history row.
--
-- Same LEFT JOINs and same ordering expression as ListSessionSets above, for the
-- same reason: an INNER JOIN here would quietly drop assistance work from the
-- history list while the session detail still showed it. Keep the two in sync.
-- name: ListSessionExerciseWeights :many
SELECT ss.session_id,
       e.name                      AS exercise_name,
       COUNT(ss.id)                AS set_count,
       MAX(ss.target_reps)::int    AS reps,
       MAX(ss.weight_lb)::numeric  AS weight_lb
FROM session_sets ss
JOIN exercises e ON e.id = ss.exercise_id
JOIN sessions s ON s.id = ss.session_id
LEFT JOIN program_day_exercises pde
  ON pde.program_day_id = s.program_day_id AND pde.exercise_id = ss.exercise_id
LEFT JOIN program_day_assistance pda
  ON pda.program_day_id = s.program_day_id
 AND pda.exercise_id = ss.exercise_id
 AND pda.user_id = s.user_id
WHERE ss.session_id = ANY(@session_ids::int[])
  AND s.user_id = sqlc.arg('user_id')::int
GROUP BY ss.session_id, e.name
ORDER BY ss.session_id, MIN(COALESCE(pde.position, 1000 + pda.position, 2000));

-- ListSessionPersonalBests returns, for each lift in the given session, the
-- heaviest weight that lifter has ever worked on it in ANY OTHER session.
--
-- This is what the active-session screen compares a logged set against to
-- decide it is a personal record. It used to be one history request per
-- distinct lift in the session — five or six round trips, each returning a
-- whole training history, to end up with one number apiece.
--
-- The current session is excluded rather than filtered client-side, which is
-- what makes the answer stable: the browser previously took the maximum over
-- everything it could see, so a set already logged in THIS session counted
-- towards the record it was being compared against, and what qualified as a PR
-- depended on when the page happened to be opened.
--
-- actual_reps > 0 is the same definition of real work as ListExerciseHistory,
-- so a weight only defends a record once it has actually been performed.
--
-- Both bests are carried, because a heavier bar is not the only way to set a
-- record: the same weight for more reps is a higher estimated max, and on a 5x5
-- that is exactly what the session before a jump looks like. The weight alone
-- was all the live screen needed to fire confetti, but the recap reports both
-- kinds — and when it falls back to reconstructing itself from this response
-- offline, a weight-only answer made it quietly report fewer records than the
-- server would.
--
-- The estimate comes from e1rm_lb() (migration 0019), shared with
-- RecapExerciseBaseline and RackedExerciseBaseline. Set.E1RM in Go rounds to the
-- pound and compares straight against these numbers, so a formula that differed
-- between the three would put the two sides at odds inside a sub-pound band —
-- which is exactly where a record is decided.
-- name: ListSessionPersonalBests :many
SELECT ss.exercise_id,
       MAX(ss.weight_lb)::numeric AS best_weight_lb,
       MAX(e1rm_lb(ss.weight_lb, ss.actual_reps))::numeric AS best_e1rm_lb
FROM session_sets ss
JOIN sessions s ON s.id = ss.session_id
WHERE s.user_id = sqlc.arg('user_id')::int
  AND ss.session_id <> sqlc.arg('session_id')
  AND ss.actual_reps > 0
  AND ss.exercise_id IN (
      SELECT exercise_id FROM session_sets WHERE session_id = sqlc.arg('session_id')
  )
GROUP BY ss.exercise_id;
