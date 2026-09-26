-- How much training every lifter on the install has behind them.
--
-- Install-wide and unscoped, unlike everything in sessions.sql: a level is drawn
-- beside a name wherever a name appears, so it is exactly as public as the name
-- is. This is the read a client makes once and then consults per name rendered,
-- for the reason houses.sql and achievements.sql are shaped the same way — asking
-- per lifter would be a request per row of the feed.
--
-- The count is the whole feature: experience is a flat 100 XP per qualifying
-- session (see internal/levels), so a session count and an XP total are the same
-- number in different units, and nothing is stored to keep them agreeing.
--
-- WHY THIS IS NOT A TABLE
--
-- The crowns next door are reconciled into lifter_achievements because deriving
-- one costs the heaviest query in the app. This costs a correlated count over an
-- indexed column, and it is read once per client per poll rather than per name
-- rendered — so the table would buy nothing and cost the two things a stored
-- counter always costs: a session edited after the fact would leave it stale, and
-- a session deleted would leave experience behind that no session explains.
--
-- WHAT COUNTS, AND WHY IT IS SPELLED OUT TWICE
--
-- A session earns its 100 XP when it is over AND somebody actually lifted in it.
--
-- "Over" is the rule at the top of sessions.sql, copied here because that file
-- says to keep the copies in sync rather than to share them — a generated column
-- cannot call now(). THIS FILE IS ONE OF THE COPIES. Sessions that age out matter
-- here more than anywhere else: a lifter who trains and forgets to tap Finish has
-- still trained, and never writing finished_at must not cost them the session.
--
-- The EXISTS is the guard that rule needs on its own. Without it, opening a
-- session and logging nothing pays out 100 XP twelve hours later, which would make
-- the badge a measure of tabs opened. It is the same filter last_trained_on
-- applies in users.sql, and for the same reason. Note it asks actual_reps > 0
-- rather than `completed` — `completed` means the set HIT ITS TARGET, so requiring
-- it would quietly stop paying any lifter who missed a rep.
--
-- EVERY ACCOUNT IS LISTED, including one that has never trained, which comes out
-- at zero and is level 1. The alternative was leaving them out and letting the
-- client read the absence as level 1, and it is worse twice over: absence would
-- also be the answer for an id this install does not have, so the two would be
-- indistinguishable, and "a lifter starts at level 1 with no experience" would
-- have to be written down a second time in TypeScript — when the whole point of
-- computing the curve server-side is that it has one owner. The payload is one
-- row per account, bounded by a number the install's owner sets by hand.
--
-- FROM users rather than a GROUP BY over sessions for that reason, and the
-- correlated subquery is ListLifters' own shape in this same directory — at a
-- handful of accounts the planner runs it once per row against sessions_user_idx.
-- name: ListLifterLevels :many
SELECT u.id AS user_id,
       (
         SELECT COUNT(*)
         FROM sessions s
         WHERE s.user_id = u.id
           AND (s.finished_at IS NOT NULL
                OR s.created_at < now() - INTERVAL '12 hours')
           AND EXISTS (
             SELECT 1 FROM session_sets ss
             WHERE ss.session_id = s.id AND ss.actual_reps > 0
           )
       )::int AS qualifying_sessions
FROM users u
ORDER BY u.id;

-- How much training ONE lifter has behind them.
--
-- The per-lifter form of ListLifterLevels above, for the path that needs one answer
-- rather than the install's: finishing a session, where what matters is whether that
-- session took this lifter past a rung. Reading the whole install to answer for one
-- of them would be the wrong shape on the request every workout ends with.
--
-- The predicate is the same one, copied rather than shared because the two differ in
-- everything else — one is grouped over every account, this is a scalar for one — and
-- because there is no way to share a WHERE clause between two sqlc queries. THAT
-- MAKES THIS THE FOURTH COPY of the "over" expression that sessions.sql asks to be
-- kept in sync, and that file names it.
-- name: CountQualifyingSessionsForLifter :one
SELECT COUNT(*)::int AS qualifying_sessions
FROM sessions s
WHERE s.user_id = sqlc.arg('user_id')::int
  AND (s.finished_at IS NOT NULL OR s.created_at < now() - INTERVAL '12 hours')
  AND EXISTS (SELECT 1
              FROM session_sets ss
              WHERE ss.session_id = s.id
                AND ss.actual_reps > 0);
