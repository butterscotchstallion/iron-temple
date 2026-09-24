-- Applause and conversation on a session.
--
-- Every other query in this directory that touches a session is scoped by
-- user_id, because until now every one of them was answering a question about the
-- caller's own training. These are not, and that difference is the whole reason
-- this file exists separately.
--
-- THE TRAP THIS FILE EXISTS TO AVOID
--
-- GetSession takes (id, user_id) and is what enforces ownership everywhere else:
-- another lifter's session id returns no rows and the handler turns that into a
-- 404. Reusing it here would 404 on exactly the sessions this feature is FOR —
-- reacting to your own workout is the case that barely matters. So SessionExists
-- below resolves a session without scoping by owner, and it is the only query in
-- the codebase that deliberately does so.
--
-- What that costs has to be paid back somewhere, and the answer is that it costs
-- nothing here: every account on this install may already read every other
-- account's sessions through /lifters, so a session id is not a secret and
-- confirming one exists reveals nothing. If a visibility model is ever added,
-- this is one of the two places that has to learn about it (the other is
-- ListFeedSessions).

-- SessionExists resolves a session id to its OWNER, without scoping by the
-- caller.
--
-- Returns the owner rather than a bare boolean because callers need it: a
-- reaction is refused on your own session, and knowing that means knowing whose
-- it is. One round trip either way.
-- name: SessionExists :one
SELECT id, user_id FROM sessions WHERE id = sqlc.arg('id');

-- ---- reactions ----

-- AddSessionReaction records one lifter's applause.
--
-- ON CONFLICT DO NOTHING rather than an error: the primary key makes a repeat tap
-- a no-op, which is what a lifter tapping twice means. Returning a 409 for it
-- would be reporting a mistake nobody made, and checking first would be a race
-- under concurrent taps that the constraint settles for free.
--
-- :execrows because 0026 needs to tell those two apart. The lifter sees 204
-- either way, but a repeat tap must not mint a second notification, and with
-- ON CONFLICT DO NOTHING the row count is the only thing that knows. Under
-- concurrent taps exactly one of them reports a row, which is the same
-- guarantee the primary key is already giving.
-- name: AddSessionReaction :execrows
INSERT INTO session_reactions (session_id, user_id, emoji)
VALUES (sqlc.arg('session_id')::int, sqlc.arg('user_id')::int, sqlc.arg('emoji'))
ON CONFLICT (session_id, user_id, emoji) DO NOTHING;

-- RemoveSessionReaction withdraws it. :execrows so the handler can tell a
-- withdrawal from a tap at something that was never there — though both answer
-- 204, since the end state the caller asked for is the same either way.
-- name: RemoveSessionReaction :execrows
DELETE FROM session_reactions
WHERE session_id = sqlc.arg('session_id')::int
  AND user_id = sqlc.arg('user_id')::int
  AND emoji = sqlc.arg('emoji');

-- ListSessionReactions is one session's applause, grouped by emoji, with whether
-- the caller is among each group.
--
-- Grouped in SQL rather than in Go because the count is all the surface draws —
-- a row per reaction would send N rows to render one number, and on a session
-- three lifters applauded that is three times the payload for the same "3".
--
-- `mine` is what lets the UI show the caller's own taps as pressed, and it is a
-- bool_or over the group rather than a second query: asking "which of these did I
-- give" separately would be a second round trip whose answer has to be joined
-- back onto this one anyway.
--
-- Ordered by count and then by emoji, so the strongest reaction leads and ties
-- resolve the same way on every request. Without the emoji tiebreak a session
-- with two 💪 and two 🔥 would reorder itself between page loads.
-- name: ListSessionReactions :many
SELECT emoji,
       COUNT(*)::bigint AS total,
       bool_or(user_id = sqlc.arg('viewer_id')::int) AS mine
FROM session_reactions
WHERE session_id = sqlc.arg('session_id')::int
GROUP BY emoji
ORDER BY total DESC, emoji;

-- ---- comments ----

-- AddSessionComment posts one. The body arrives already trimmed and length
-- checked — see maxCommentBody in internal/api — because "is this comment
-- acceptable" is a product rule and belongs where the error message is written.
-- name: AddSessionComment :one
INSERT INTO session_comments (session_id, user_id, body)
VALUES (sqlc.arg('session_id')::int, sqlc.arg('user_id')::int, sqlc.arg('body'))
RETURNING id, session_id, user_id, body, created_at;

-- ListSessionComments is the conversation on one session, oldest first.
--
-- Joined to users so a comment arrives with its author, which is the only way it
-- is ever rendered. avatar_etag comes along for the reason ListLifters carries
-- it: the surface draws an avatar per comment, and a lookup per row would be a
-- query per comment.
--
-- Oldest first, unlike every other list in this schema. A conversation reads
-- downwards — a reply under the thing it replies to — where a history reads
-- newest first because the recent session is the one you want.
--
-- SELECTED FROM ONE END AND ORDERED FROM THE OTHER, which is the only subtle
-- thing here. The inner query takes the NEWEST page — that is what offset counts
-- back through — and the outer ORDER BY turns it the right way up for reading.
-- Paging from the oldest end instead would mean a lifter opening a session with
-- forty comments on it lands at the beginning of a conversation whose last line
-- is the one they were told about.
--
-- The subquery selects ids only, so the joins that decorate a comment with its
-- author run over one page rather than over the whole thread.
--
-- Until this was paged it was the one unbounded list in the app, and the
-- generated-activity scheduler adds comments to sessions every day.
-- name: ListSessionComments :many
SELECT c.id,
       c.session_id,
       c.body,
       c.created_at,
       u.id   AS user_id,
       u.username,
       u.display_name,
       u.avatar_color,
       COALESCE(ua.etag, '') AS avatar_etag
FROM session_comments c
JOIN users u ON u.id = c.user_id
LEFT JOIN user_avatars ua ON ua.user_id = u.id
WHERE c.id IN (
    SELECT p.id
    FROM session_comments p
    WHERE p.session_id = sqlc.arg('session_id')::int
    ORDER BY p.created_at DESC, p.id DESC
    LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off')
)
ORDER BY c.created_at, c.id;

-- CountSessionComments is how many there are in total, so a surface showing the
-- tail of a conversation can say how much of it is above the fold.
--
-- Separate from the list rather than a window function over it, because the
-- list returns a page and this counts the thread: a COUNT(*) OVER () would be
-- computed per returned row and would still be wrong on an empty page, where
-- there are no rows to carry it.
-- name: CountSessionComments :one
SELECT COUNT(*)::bigint AS total
FROM session_comments
WHERE session_id = sqlc.arg('session_id')::int;

-- GetSessionComment reads one comment's owning session and author, which is what
-- a delete has to know before it is allowed: the author may remove their own, and
-- the admin may remove any.
--
-- session_id comes back so the handler can reject a comment id that belongs to a
-- different session than the URL names. Without that check /sessions/1/comments/9
-- would delete comment 9 wherever it actually lives, and the path would be
-- decorative.
-- name: GetSessionComment :one
SELECT id, session_id, user_id FROM session_comments WHERE id = sqlc.arg('id');

-- name: DeleteSessionComment :execrows
DELETE FROM session_comments WHERE id = sqlc.arg('id');
