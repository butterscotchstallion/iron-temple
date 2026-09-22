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

-- ListNotifications is one lifter's panel, newest first.
--
-- Every join but the first is a LEFT JOIN, because the subject of a
-- notification is optional by kind: 'joined' has no session, and only a
-- 'comment' or 'reply' has a comment. The actor join is inner — a row whose
-- actor has been deleted cannot be rendered, and the cascade means one cannot
-- exist anyway.
--
-- avatar_etag rides along for the reason ListSessionComments carries it: the
-- panel draws an avatar per row, and a lookup per row would be a query per
-- notification.
--
-- The comment body comes back whole rather than truncated. Where to cut it is a
-- rendering decision that depends on how wide the panel is, and 0024 already
-- caps a comment at 256 runes, so "whole" is bounded.
-- name: ListNotifications :many
SELECT n.id,
       n.kind,
       n.session_id,
       n.comment_id,
       n.emoji,
       n.created_at,
       n.read_at,
       u.id   AS actor_id,
       u.username,
       u.display_name,
       u.avatar_color,
       COALESCE(ua.etag, '') AS avatar_etag,
       s.user_id AS session_owner_id,
       pd.name   AS program_day_name,
       c.body    AS comment_body
FROM notifications n
JOIN users u ON u.id = n.actor_id
LEFT JOIN user_avatars ua ON ua.user_id = u.id
LEFT JOIN sessions s ON s.id = n.session_id
LEFT JOIN program_days pd ON pd.id = s.program_day_id
LEFT JOIN session_comments c ON c.id = n.comment_id
WHERE n.user_id = sqlc.arg('user_id')::int
ORDER BY n.created_at DESC, n.id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- CountUnreadNotifications is the badge.
--
-- Asked on every poll by every signed-in client, and answered far more often
-- than the list is read — which is what notifications_unread_idx is for.
-- name: CountUnreadNotifications :one
SELECT COUNT(*)::bigint AS total
FROM notifications
WHERE user_id = sqlc.arg('user_id')::int
  AND read_at IS NULL;

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
  AND read_at IS NULL;

-- ClearNotifications is "clear all", and it deletes.
--
-- See 0026 for why there is no cleared_at to set instead. Nothing rebuilds
-- these rows, so a cleared notification stays gone.
-- name: ClearNotifications :exec
DELETE FROM notifications WHERE user_id = sqlc.arg('user_id')::int;

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
-- name: CreateReactionNotification :exec
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
  AND s.user_id <> sqlc.arg('actor_id')::int;

-- DeleteReactionNotification withdraws the notification along with the
-- applause.
--
-- Matched on the actor, the session and the emoji rather than on an id, because
-- 0024 gave session_reactions a composite primary key and no surrogate to
-- reference. Those three columns identify the row the same way that primary key
-- does — the recipient is whoever owns the session, which is not a free
-- variable — so naming user_id as well would add nothing.
--
-- Deliberately unconditional about what it finds: withdrawing applause that was
-- never given removes nothing and reports nothing, exactly as
-- RemoveSessionReaction does.
-- name: DeleteReactionNotification :exec
DELETE FROM notifications
WHERE kind = 'reaction'
  AND actor_id = sqlc.arg('actor_id')::int
  AND session_id = sqlc.arg('session_id')::int
  AND emoji = sqlc.arg('emoji')::text;

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
-- name: CreateCommentNotifications :exec
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
         CASE recipient.kind WHEN 'comment' THEN 0 ELSE 1 END;

-- CreateJoinNotifications announces a new account to everybody already here.
--
-- Called from all three places an account is made: self-registration, the admin
-- area, and the generated-activity roster. The last of those is the reason this
-- is one statement rather than a loop — seeding four personas announces each of
-- them inside the transaction that made it.
--
-- No session, no comment, no emoji; 'joined' is the kind whose subject is the
-- actor themselves.
-- name: CreateJoinNotifications :exec
INSERT INTO notifications (user_id, actor_id, kind)
SELECT u.id, sqlc.arg('actor_id')::int, 'joined'::text
FROM users u
WHERE u.id <> sqlc.arg('actor_id')::int;
