-- Achievements: the catalogue, the ledger of reigns, and the reconciler's writes.
--
-- The reads split by who is asking about whom, and the split is the whole point
-- of the feature's shape. ListCurrentAchievementHolders is asked by every
-- signed-in client about EVERYBODY, because a crown has to be drawable beside any
-- name the app renders; ListLifterAchievements is asked about ONE lifter when
-- their profile is open. The first is small and hot and answered by a partial
-- index; the second is the only one that reads history.
--
-- Nothing here is scoped to a caller, and that is deliberate rather than an
-- omission. Achievements are public on the install by construction — the crown
-- exists to be seen by other people, and the leaderboard it is derived from
-- already lists every lifter to every signed-in account.
--
-- The writes are the reconciler's, and they are a DIFF, not a rebuild. See
-- refreshCrowns in internal/api/achievements.go for why: a reign is a continuous
-- stretch, so the pass that runs hourly must leave an unchanged holder's row
-- exactly as it found it.

-- ---- reading ----

-- ListAchievements is the catalogue: everything that can be earned.
--
-- Ordered by sort_order so every surface lists them the way buildBoards orders
-- the boards they come from, with volume last.
-- name: ListAchievements :many
SELECT slug,
       kind,
       metric,
       label,
       description,
       sort_order
FROM achievements
ORDER BY sort_order, slug;

-- ListCurrentAchievementHolders is who is wearing what, right now.
--
-- THIS IS THE SITE-WIDE READ. Every signed-in client asks it and then draws from
-- the answer beside every name on the feed, the roster, the leaderboard, the
-- notification panel and the header. It has to be cheap, and it is: 0032's
-- partial index holds only the open reigns, so this is bounded by the account
-- count times the size of the catalogue rather than by the ledger's history.
--
-- SEVERAL LIFTERS CAN HOLD ONE ACHIEVEMENT. The leaderboard gives tied figures
-- the same rank, so two lifters genuinely level on a board are both first and
-- both appear here under the same slug. A client must group these rather than
-- assume one row per achievement.
--
-- The lifter's own columns ride along for the reason ListNotificationGroups
-- carries avatar_etag: the caller draws a person, and a lookup per holder would
-- be a query per crown.
-- name: ListCurrentAchievementHolders :many
SELECT la.achievement_slug,
       la.held_from,
       u.id AS user_id,
       u.username,
       u.display_name,
       u.avatar_color,
       COALESCE(ua.etag, '') AS avatar_etag
FROM lifter_achievements la
JOIN achievements a ON a.slug = la.achievement_slug
JOIN users u ON u.id = la.user_id
LEFT JOIN user_avatars ua ON ua.user_id = u.id
WHERE la.held_until IS NULL
ORDER BY a.sort_order, u.id;

-- ListLifterAchievements is one lifter's profile section: what they hold now and
-- what they have held before.
--
-- FOLDED TO ONE ROW PER ACHIEVEMENT, with the reign count on it. That is what
-- makes "held this three times" a number rather than a list the client has to
-- count, and it keeps a lifter who has traded a crown back and forth for a year
-- from returning a row per exchange.
--
-- held_now leads the ordering because a crown someone is wearing is the thing
-- they are being looked at for; past reigns sort behind it by the catalogue's own
-- order rather than by recency, so the section reads the same way round every
-- time it is opened.
--
-- first_held_from and last_held_from are both returned because they answer
-- different questions — "since when" for a current reign, "when last" for a
-- lapsed one — and the client picks by held_now rather than asking twice.
-- name: ListLifterAchievements :many
SELECT a.slug,
       a.kind,
       a.metric,
       a.label,
       a.description,
       a.sort_order,
       COUNT(*)::bigint AS times_held,
       bool_or(la.held_until IS NULL) AS held_now,
       MIN(la.held_from)::timestamptz AS first_held_from,
       MAX(la.held_from)::timestamptz AS last_held_from
FROM lifter_achievements la
JOIN achievements a ON a.slug = la.achievement_slug
WHERE la.user_id = sqlc.arg('user_id')::int
GROUP BY a.slug, a.kind, a.metric, a.label, a.description, a.sort_order
ORDER BY held_now DESC, a.sort_order, a.slug;

-- ---- the reconciler's two writes ----
--
-- There is deliberately no "read the open reigns first" query to go with these.
-- Between them they express the whole diff: the close says "everybody except
-- these" and the open is an upsert, so neither needs a before-picture and a pass
-- costs two statements per board rather than three.

-- OpenAchievementReign starts a reign, or does nothing if one is already open.
--
-- The ON CONFLICT is what makes the hourly pass idempotent, and it is what keeps
-- a continuous reign as ONE row: a lifter who has been top all month already has
-- an open reign, this does nothing, and their held_from still says when they took
-- it rather than when the sweeper last ran.
--
-- The conflict target repeats the index's WHERE clause because the index is
-- partial — Postgres will not infer a partial index from the columns alone.
--
-- :execrows so the caller learns whether this was a genuinely NEW reign. Only a
-- new one is news, and only a new one raises notifications; without this the
-- install would announce the same crown every hour, forever.
-- name: OpenAchievementReign :execrows
INSERT INTO lifter_achievements (user_id, achievement_slug)
VALUES (sqlc.arg('user_id')::int, sqlc.arg('achievement_slug')::text)
ON CONFLICT (achievement_slug, user_id) WHERE held_until IS NULL
DO NOTHING;

-- CloseAchievementReignsExcept ends every open reign on one achievement that is
-- not held by one of the lifters named.
--
-- Expressed as "everybody except these" rather than "close this lifter's" because
-- that is the shape of the fact the reconciler actually has: it knows who leads
-- the board now, and it does not separately know who used to. One statement per
-- board then handles every way a reign can end — dethroned by somebody else,
-- overtaken into a tie that no longer includes them, or dropping off the board
-- entirely because the metric stopped being defined for them.
--
-- Passing an EMPTY array is meaningful and correct: it closes every open reign on
-- that achievement, which is what should happen when a board has no entries at
-- all. `<> ALL` on an empty array is true for every row, so this needs no special
-- case in Go.
-- name: CloseAchievementReignsExcept :execrows
UPDATE lifter_achievements
SET held_until = now()
WHERE achievement_slug = sqlc.arg('achievement_slug')::text
  AND held_until IS NULL
  AND user_id <> ALL (sqlc.arg('holder_ids')::int[]);

-- ---- notifications ----

-- CreateCrownNotifications tells the lifter who took a crown, and anybody who
-- follows them.
--
-- THIS USED TO GO TO THE WHOLE INSTALL AND NOT TO THE HOLDER, and the reversal is
-- deliberate enough to record rather than quietly overwrite. The old comment argued
-- that "a notification saying you did the thing you are looking at is noise", which
-- was true while the crown reached everybody else and the holder had the ornament on
-- their own name to learn from. It stopped being true the moment the panel became
-- yours-and-the-people-you-chose: a lifter who follows nobody would have had a
-- feature that never once spoke to them.
--
-- So the rule is now the two audiences that have a reason to care. The holder,
-- always — it is their achievement, and the row is what survives a missed toast and
-- what opens the achievement dialog. And their followers, because following is the
-- install's way of saying "tell me about this person".
--
-- NOTE WHAT THIS BREAKS, in the table's own terms: notifications.actor_id is
-- NOT NULL and 0026 wrote that every insert filters the actor out of the
-- recipients, "because being told about your own applause is noise". That holds for
-- applause and no longer holds here — this is the first kind whose recipient may be
-- its own actor. The column is nullable-free and unconstrained, so nothing in the
-- schema had to change, but a reader of 0026 should know one kind now disagrees
-- with it.
--
-- Everybody else on the install is told nothing. That is not a visibility rule —
-- every crown stays on the leaderboard and beside every name, exactly as before —
-- it is only about what gets pushed. See 0033.
--
-- No session, no comment, no emoji. achievement_slug is this kind's subject, and
-- it is what lets the panel name WHICH board was won.
-- Returns everybody told, so the caller can wake their sockets.
-- name: CreateCrownNotifications :many
INSERT INTO notifications (user_id, actor_id, kind, achievement_slug)
SELECT u.id,
       sqlc.arg('actor_id')::int,
       'crown'::text,
       sqlc.arg('achievement_slug')::text
FROM users u
WHERE u.id = sqlc.arg('actor_id')::int
   OR EXISTS (
     SELECT 1 FROM follows f
     WHERE f.followee_id = sqlc.arg('actor_id')::int
       AND f.follower_id = u.id
   )
RETURNING user_id;
