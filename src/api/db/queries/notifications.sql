-- The notification panel, and the writes that fill it.
--
-- Reads here are scoped by recipient — sqlc.arg('user_id') is always the
-- caller, and there is no query in this file that returns another account's
-- notifications. That is the whole access model: a row names who it is for, and
-- nothing reads one that is not yours.
--
-- The writes are the opposite shape and worth reading as a group. Every one of
-- them decides its RECIPIENTS IN SQL rather than taking them as a parameter,
-- and that is deliberate: the same fan-out has to happen from the reaction and
-- comment handlers AND from the generated-activity scheduler, which writes in
-- bulk inside its own transaction (see generateRecognition in internal/api).
-- Two Go call sites computing "who should hear about this" separately is two
-- chances to disagree. An INSERT ... SELECT is one rule, and it runs inside
-- whichever transaction the caller already has.

-- ---- reading ----

-- ListNotificationGroups is one lifter's panel, newest first — ONE ROW PER
-- THING THAT HAPPENED rather than one row per notification.
--
-- WHY THIS IS GROUPED, AND WHY HERE
--
-- The writes below deliberately fan one event out to a row per recipient, and
-- three lifters applauding one session is three rows that read almost
-- identically. The generated-activity scheduler (0025) manufactures exactly that
-- shape every day. Unfolded, the panel's twenty rows get spent saying the same
-- sentence with different names in it.
--
-- Folding it in the CLIENT was the obvious cheaper option and it is wrong: the
-- LIMIT would still apply to the rows, so the noisiest install would draw the
-- FEWEST rows — twenty applause on one session collapsing to a single line and
-- an empty panel beneath it. Grouping has to happen where the LIMIT is, which is
-- here. ListSessionReactions in recognition.sql grouped in SQL for the adjacent
-- reason and is the precedent.
--
-- THE GROUP KEY IS (kind, session_id)
--
-- One group per session per kind. 'comment' and 'reply' stay separate groups on
-- the same session because they are different sentences about different
-- relationships to it, and a lifter never gets both for one comment anyway.
-- 'joined' has no session at all, and GROUP-style partitioning treats those
-- NULLs as equal, so every new account on the install folds into a single row —
-- which is the case that matters most on a freshly seeded one, where the roster
-- announces four personas at once.
--
-- A group spans the whole live window (0030 archives at a month). So applause
-- from three weeks ago folds in with applause from this morning, and the group
-- sorts by its NEWEST member — an old group floats back to the top when
-- somebody adds to it. That is correct: the new applause is news, and the row
-- that carries it is the row that says who else is in it.
--
-- HOW ONE ROW IS BUILT
--
-- The CTE runs one pass over the caller's live rows with a single window: the
-- partition is the group, and the ordering inside it is the panel's ordering.
-- rn = 1 is the group's newest member, and it is the row whose scalar columns
-- the panel draws — the emoji, the comment, the workout name, the actor. So a
-- group of three comments quotes and links to the most recent one; the rest are
-- on the session the row points at, which is where a conversation belongs.
--
-- ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING IS LOAD-BEARING.
-- An ORDER BY inside a window makes the default frame end at the current row,
-- which turns every aggregate below into a RUNNING one — and since the row this
-- query keeps is rn = 1, each of them would see a partition of exactly one. The
-- counts would all read 1 and the fold would silently not happen. The explicit
-- frame is what lets the newest row carry facts about the whole group.
--
-- actor_ids AND actor_names ARE PARALLEL ARRAYS, newest-first, WITH DUPLICATES.
-- Two things to know about that:
--
--   - Postgres has no DISTINCT for window aggregates (no count(DISTINCT x) OVER
--     w, no array_agg(DISTINCT x) OVER w), so the duplicates are removed once,
--     in Go, walking the two arrays together — see groupActors in
--     internal/api/notifications.go. One lifter who applauded with two emoji is
--     two rows here and must still be one person in the sentence.
--   - Deduping happens by ID and not by name, so two accounts that chose the
--     same display name are two people. Names are for reading; ids are for
--     counting.
--
-- They are aggregated in the window's order, so element 1 is the rn = 1 row's
-- own actor — the one the panel already names from the columns below. Go drops
-- it and keeps what is left.
--
-- read_at IS THE GROUP'S, NOT THE REPRESENTATIVE'S. A group reports itself read
-- only once every member has been read, and then with the LATEST of those
-- stamps. Taking the newest member's read_at instead would let a group whose
-- newest row had been opened hide an older unread one — invisible in the panel
-- and still counted by the badge, which is the one disagreement between those
-- two surfaces that must not exist. On a group of one this is byte-identical to
-- what the column held.
--
-- Every join but the first is still a LEFT JOIN, because the subject of a
-- notification is optional by kind. The actor join is inner — a row whose actor
-- has been deleted cannot be rendered, and the cascade means one cannot exist.
-- avatar_etag rides along for the reason ListSessionComments carries it: the
-- panel draws an avatar per row, and a lookup per row would be a query per
-- notification. The comment body comes back whole rather than truncated, since
-- where to cut it depends on how wide the panel is and 0024 already caps a
-- comment at 256 runes.
--
-- THE COST, plainly: this cannot stop at LIMIT rows. Knowing which twenty
-- GROUPS are newest means folding every live row the caller has, because the
-- sort key of the output is computed from the rows being folded. No index fixes
-- that, which is why 0031 adds none for it — the bound is the retention window
-- instead, and a month of a household install's notifications is hundreds of
-- rows.
-- name: ListNotificationGroups :many
WITH live AS (
    SELECT n.id,
           n.kind,
           n.session_id,
           n.comment_id,
           n.emoji,
           n.created_at,
           n.actor_id,
           n.achievement_slug,
           -- Whether the whole group names ONE achievement, which is what
           -- decides if the row above may be spoken aloud.
           --
           -- 'crown' has no session, so 0031's (kind, session_id) key folds every
           -- crown on the install into a single group — across boards. The
           -- representative's slug is therefore not the group's the way emoji and
           -- comment_id are: a row reading "took the crown on Volume" while
           -- folding four other boards is a specific claim the group does not
           -- support. So the client is handed the slug and permission to use it,
           -- and says the unnamed thing when it has one without the other.
           --
           -- min = max is how a window asks "are these all the same": Postgres
           -- has no count(DISTINCT x) OVER w, which is the same gap that puts
           -- actor deduping in Go. On a group of one it is trivially true, which
           -- is the common case and the one that gets to name its board.
           -- IS NOT DISTINCT FROM rather than =, so a group of all-NULLs — every
           -- other kind — compares equal rather than unknown, and those rows have
           -- no slug to offer anyway.
           (min(n.achievement_slug) OVER w
                IS NOT DISTINCT FROM max(n.achievement_slug) OVER w)::bool
               AS one_achievement,
           row_number() OVER w AS rn,
           -- Whether anything in the group is still unread, which is both the
           -- panel's dot and what makes the group count towards the badge.
           bool_or(n.read_at IS NULL) OVER w AS has_unread,
           max(n.read_at) OVER w AS last_read_at,
           array_agg(n.actor_id) OVER w AS actor_ids,
           -- The name the panel would print for this actor, decided here rather
           -- than in Go so the fallback to username lives in one place. The
           -- outer query still returns display_name and username for the
           -- representative, because the row also draws that actor's avatar and
           -- links to their profile.
           array_agg(COALESCE(NULLIF(u.display_name, ''), u.username)) OVER w
               AS actor_names
    FROM notifications n
    JOIN users u ON u.id = n.actor_id
    WHERE n.user_id = sqlc.arg('user_id')::int
      AND n.archived_at IS NULL
    WINDOW w AS (
        PARTITION BY n.kind, n.session_id
        ORDER BY n.created_at DESC, n.id DESC
        ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING
    )
)
SELECT g.id,
       g.kind,
       g.session_id,
       g.comment_id,
       g.emoji,
       g.achievement_slug,
       g.one_achievement,
       g.created_at,
       (CASE WHEN g.has_unread THEN NULL ELSE g.last_read_at END)::timestamptz
           AS read_at,
       g.actor_ids::int[]    AS actor_ids,
       g.actor_names::text[] AS actor_names,
       u.id   AS actor_id,
       u.username,
       u.display_name,
       u.avatar_color,
       COALESCE(ua.etag, '') AS avatar_etag,
       s.user_id AS session_owner_id,
       pd.name   AS program_day_name,
       c.body    AS comment_body
FROM live g
JOIN users u ON u.id = g.actor_id
LEFT JOIN user_avatars ua ON ua.user_id = u.id
LEFT JOIN sessions s ON s.id = g.session_id
LEFT JOIN program_days pd ON pd.id = s.program_day_id
LEFT JOIN session_comments c ON c.id = g.comment_id
WHERE g.rn = 1
ORDER BY g.created_at DESC, g.id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- CountUnreadNotifications is the badge, and it counts GROUPS.
--
-- It has to, now that the panel folds. Twelve applause on one session draw one
-- row, so they are one unread thing — a badge reading 12 over a single row is a
-- number nobody can reconcile with what they are looking at, and "mark all
-- read" taking it from 12 to 0 in one tap would look like a bug rather than like
-- the one thing it did.
--
-- A group counts as unread if ANY member is, which is the same rule the panel's
-- dot draws from. The two surfaces agree by construction because they group on
-- the same key.
--
-- Asked on every poll by every signed-in client and answered far more often than
-- the list is read, which is what 0031 reshaped notifications_unread_idx for:
-- (user_id, kind, session_id) makes this index-only and delivers the groups
-- already adjacent, so it aggregates a sorted scan rather than hashing
-- everything unread.
-- name: CountUnreadNotifications :one
SELECT COUNT(*)::bigint AS total
FROM (
    SELECT 1
    FROM notifications
    WHERE user_id = sqlc.arg('user_id')::int
      AND read_at IS NULL
      AND archived_at IS NULL
    GROUP BY kind, session_id
) AS grp;

-- ---- the two buttons ----

-- MarkNotificationsRead is "mark all read".
--
-- Restricted to rows that are still unread so a second tap updates nothing
-- rather than rewriting every row's timestamp to a later now(). The stamp is
-- when it was FIRST read, which is the only reading of it that means anything.
-- name: MarkNotificationsRead :exec
UPDATE notifications
SET read_at = now()
WHERE user_id = sqlc.arg('user_id')::int
  AND read_at IS NULL
  AND archived_at IS NULL;

-- MarkNotificationGroupRead is one ROW OF THE PANEL, for a lifter who followed
-- it through to the thing it was about.
--
-- Takes the id ListNotificationGroups returned, which is the group's newest
-- member, and marks everything that row folded. That is not a widening of what
-- the endpoint meant — it is the same meaning against the new unit. A row in the
-- panel says "Bob, Cara and 4 others applauded your Push Day", and a lifter who
-- opens it has been told about all six. Leaving the other five unread would put
-- the row straight back with a dot on it and no way to ever clear it except
-- "mark all read", which is the blunt instrument this endpoint exists to avoid.
--
-- The group is resolved FROM THE ID rather than passed in as a key, so the
-- client never has to know what the grouping rule is. It sends back an id it was
-- given; the rule stays in SQL, in one place, shared with the read above.
--
-- self-joined with UPDATE ... FROM: g is the row the caller named, n is every
-- live row in its group. session_id IS NOT DISTINCT FROM rather than = because
-- the 'joined' group's session_id is NULL on both sides, and = would match
-- nothing there — the one group where a plain equality silently does nothing.
--
-- STILL SCOPED TO THE CALLER INSIDE THE STATEMENT, which is the whole access
-- control. n.user_id = g.user_id and g.user_id = the caller, so the update
-- cannot leave the caller's own notifications however the group key falls. There
-- is no separate "is this yours" read for a handler to forget and no 403 to
-- write: another account's id matches no row, and the answer is the same 204 as
-- marking one that was already read. Reporting 404 would confirm the id exists,
-- which is a question this endpoint has no reason to answer.
--
-- g is required to be live as well. An archived id cannot have come from the
-- panel, and resolving a group through one would stamp live rows on the strength
-- of a row nobody can see.
--
-- Restricted to unread rows for the reason MarkNotificationsRead is: read_at is
-- when a notification was FIRST read, and a second tap must not move it.
-- name: MarkNotificationGroupRead :execrows
UPDATE notifications n
SET read_at = now()
FROM notifications g
WHERE g.id = sqlc.arg('id')::int
  AND g.user_id = sqlc.arg('user_id')::int
  AND g.archived_at IS NULL
  AND n.user_id = g.user_id
  AND n.kind = g.kind
  AND n.session_id IS NOT DISTINCT FROM g.session_id
  AND n.read_at IS NULL
  AND n.archived_at IS NULL;

-- ClearNotifications is "clear all", and it deletes.
--
-- See 0026 for why there is no cleared_at to set instead. Nothing rebuilds
-- these rows, so a cleared notification stays gone.
-- name: ClearNotifications :exec
DELETE FROM notifications WHERE user_id = sqlc.arg('user_id')::int;

-- ---- retention ----

-- ArchiveOldNotifications takes the rows nobody can reach any more out of sight.
--
-- Why this exists: the generated-activity scheduler (0025) applauds and comments
-- as its personas every day, and each of those raises a row in a real lifter's
-- panel. Nothing ever read the old ones — the panel holds one page and offers no
-- way back past it — so without this the table grows at a steady rate forever
-- and every read pays for rows no surface can show.
--
-- ARCHIVES RATHER THAN DELETES, which is the decision 0030 records at length.
-- What happened on this install stays in the database; it just stops being in
-- anybody's way. Deleting would throw that away to save an index entry the
-- partial indexes do not even hold.
--
-- The window is a parameter rather than a literal so the caller owns the policy
-- and the tests can archive something without waiting a month for it.
--
-- Already-archived rows are excluded so a second pass writes nothing rather than
-- moving every archived_at forward — the same reason MarkNotificationsRead
-- restricts itself to unread rows. What archived_at records is when a row LEFT,
-- and a sweeper running hourly must not keep rewriting that.
--
-- :execrows so the sweeper can log what actually moved.
-- name: ArchiveOldNotifications :execrows
UPDATE notifications
SET archived_at = now()
WHERE created_at < now() - make_interval(days => sqlc.arg('older_than_days')::int)
  AND archived_at IS NULL;

-- ---- writing ----

-- CreateReactionNotification tells a lifter somebody applauded their session.
--
-- The recipient is looked up from the session rather than passed in, so this
-- cannot be called with the wrong one. The owner check is belt and braces — the
-- API already refuses a reaction on your own session with 403 own_session — but
-- it costs nothing and it is what makes this query safe for the generator to
-- call without repeating that rule.
--
-- Sessions predating 0005 have no owner and quietly notify nobody.
--
-- ONE LIVE ROW PER (recipient, actor, session, emoji). The NOT EXISTS below
-- and the read_at guard on DeleteReactionNotification are one mechanism and
-- have to be read together.
--
-- A repeat tap of the same emoji is already a no-op — session_reactions' primary
-- key settles it, and the handler only calls this when the insert actually
-- recorded something. What this guards is the other shape: applaud, withdraw,
-- applaud again. Each of those is a genuine new reaction, so each used to mint a
-- fresh notification at the top of the panel, which is how one lifter idly
-- toggling a button becomes somebody else's unread count climbing.
--
-- With both halves in place the worst a toggler can do is move one row's unread
-- mark on and off: the withdrawal retracts the notification only while it is
-- still unseen, and the re-applause finds the row already there and adds
-- nothing. Once it HAS been read, the row stays put and every later cycle is
-- silent.
--
-- LIVE rows only, which matters once 0030's sweeper starts archiving. Without
-- that predicate an applause from two months ago — long since archived and
-- invisible to everybody — would go on suppressing the notification for the
-- same applause given again today, and the lifter would simply never be told.
-- An archived notification is not news anybody still has; a fresh tap after it
-- is gone is.
-- RETURNS ITS RECIPIENT, which is how the live socket learns who to push to
-- without the rule for "who hears about this" moving into Go. The query still
-- decides; it now also says what it decided. It is faithful in the negative
-- case too: a self-reaction or an ownerless session matches nothing above, so
-- the silence reproduces as "no event" with no Go-side guard to forget.
-- name: CreateReactionNotification :many
INSERT INTO notifications (user_id, actor_id, kind, session_id, emoji)
SELECT s.user_id,
       sqlc.arg('actor_id')::int,
       'reaction'::text,
       s.id,
       -- Cast so the parameter types as a plain string rather than a nullable
       -- one. The column is nullable — 'joined' and the comment kinds have no
       -- emoji — but this statement never writes a NULL into it, and without
       -- the cast every caller would have to take the address of a constant.
       sqlc.arg('emoji')::text
FROM sessions s
WHERE s.id = sqlc.arg('session_id')::int
  AND s.user_id IS NOT NULL
  AND s.user_id <> sqlc.arg('actor_id')::int
  AND NOT EXISTS (
    SELECT 1 FROM notifications n
    WHERE n.kind = 'reaction'
      AND n.user_id = s.user_id
      AND n.actor_id = sqlc.arg('actor_id')::int
      AND n.session_id = sqlc.arg('session_id')::int
      AND n.emoji = sqlc.arg('emoji')::text
      AND n.archived_at IS NULL
  )
RETURNING user_id;

-- DeleteReactionNotification withdraws the notification along with the
-- applause.
--
-- Matched on the actor, the session and the emoji rather than on an id, because
-- 0024 gave session_reactions a composite primary key and no surrogate to
-- reference. Those three columns identify the row the same way that primary key
-- does — the recipient is whoever owns the session, which is not a free
-- variable — so naming user_id as well would add nothing.
--
-- Deliberately unconditional about WHETHER IT FINDS ANYTHING: withdrawing
-- applause that was never given removes nothing and reports nothing, exactly as
-- RemoveSessionReaction does.
--
-- It is not unconditional about read_at, though, and that is the half of the
-- pairing described on CreateReactionNotification above. An unread notification
-- is retracted, because nobody has been told yet and the withdrawal is still
-- private. A notification that has ALREADY BEEN READ stays: it records that
-- somebody applauded, which is a thing that happened, and deleting it deletes a
-- row the recipient has seen — from their point of view a notification silently
-- vanishing between one poll and the next, with nothing to explain it.
--
-- The applause itself is gone either way. This is only about whether the
-- telling of it is also taken back, and it can only honestly be taken back
-- before it lands.
-- Returns whose panel changed, for the same reason the insert above does: a
-- withdrawal removes a row somebody may be looking at, and they should see it
-- go rather than find out on the next poll.
-- name: DeleteReactionNotification :many
DELETE FROM notifications
WHERE kind = 'reaction'
  AND actor_id = sqlc.arg('actor_id')::int
  AND session_id = sqlc.arg('session_id')::int
  AND emoji = sqlc.arg('emoji')::text
  AND read_at IS NULL
RETURNING user_id;

-- CreateCommentNotifications fans one comment out to everybody it concerns.
--
-- Two audiences, and they are different kinds. The session's owner is told they
-- were COMMENTED on. Everybody else already talking on that session is told
-- somebody REPLIED — which is the only reason a notification ever reaches a
-- lifter about a session that is not theirs, and it is what makes a
-- conversation followable by the people in it.
--
-- DISTINCT ON collapses the overlap. An owner who has also commented on their
-- own session — allowed, and ordinary once a thread gets going — appears in
-- both halves of the union, and without this would be told twice about one
-- comment. The CASE in the ORDER BY decides which of the two survives: if you
-- own the session you hear that you were commented on, not that somebody
-- replied near you.
--
-- The author is filtered out last, which covers both halves at once: commenting
-- on your own session notifies nobody, and replying to a thread you are already
-- in does not notify you.
-- Returns every recipient it chose, so the socket can tell exactly the people
-- this query decided to tell. See CreateReactionNotification for why the answer
-- comes back from SQL rather than being recomputed in Go.
-- name: CreateCommentNotifications :many
INSERT INTO notifications (user_id, actor_id, kind, session_id, comment_id)
SELECT DISTINCT ON (recipient.user_id)
       recipient.user_id,
       sqlc.arg('actor_id')::int,
       recipient.kind,
       sqlc.arg('session_id')::int,
       sqlc.arg('comment_id')::int
FROM (
    SELECT s.user_id AS user_id, 'comment'::text AS kind
    FROM sessions s
    WHERE s.id = sqlc.arg('session_id')::int
    UNION ALL
    -- Every other comment on the session. This runs after the new comment is
    -- inserted, so it is already present here and is excluded by id — and its
    -- author is excluded again below, for the case where they had commented
    -- earlier too.
    SELECT c.user_id, 'reply'::text
    FROM session_comments c
    WHERE c.session_id = sqlc.arg('session_id')::int
      AND c.id <> sqlc.arg('comment_id')::int
) AS recipient
WHERE recipient.user_id IS NOT NULL
  AND recipient.user_id <> sqlc.arg('actor_id')::int
ORDER BY recipient.user_id,
         CASE recipient.kind WHEN 'comment' THEN 0 ELSE 1 END
RETURNING user_id;

-- CreateJoinNotifications announces a new account to everybody already here.
--
-- Called from all three places an account is made: self-registration, the admin
-- area, and the generated-activity roster. The last of those is the reason this
-- is one statement rather than a loop — seeding four personas announces each of
-- them inside the transaction that made it.
--
-- No session, no comment, no emoji; 'joined' is the kind whose subject is the
-- actor themselves.
-- Returns everybody told, which on this one is everybody on the install.
-- name: CreateJoinNotifications :many
INSERT INTO notifications (user_id, actor_id, kind)
SELECT u.id, sqlc.arg('actor_id')::int, 'joined'::text
FROM users u
WHERE u.id <> sqlc.arg('actor_id')::int
RETURNING user_id;
