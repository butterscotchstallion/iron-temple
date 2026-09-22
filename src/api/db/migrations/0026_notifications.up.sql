BEGIN;

-- What happened to you, addressed to you.
--
-- 0024 gave the install applause and conversation, but it addressed neither at
-- anybody. A reaction lands on your session and a comment lands under it, and
-- the only way to learn either had happened was to go back and look at the
-- session again. This table is the delivery half of that feature.
--
-- WHY A TABLE AND NOT A QUERY
--
-- Everything listed here could be derived: "reactions on my sessions, plus
-- comments on my sessions, newest first" is two selects and a union over tables
-- that already exist, and a single last-seen timestamp on users would carry the
-- unread count. That version was considered and rejected for one reason —
-- clearing. A derived list can only ever be cleared by moving a watermark, so
-- "clear" and "mark read" become the same gesture with different names, and a
-- lifter who clears the panel loses the unread mark on anything that arrives
-- with an older timestamp than the one they just set. A row per notification
-- makes read and cleared independent, which is what the two buttons mean.
--
-- It also decouples the feature from the shape of the thing that caused it. A
-- fifth kind — a new lift record, somebody starting your program — is a row
-- here and a sentence in the client, not another branch in a union that has to
-- stay sortable against the others.
--
-- WHAT IS NOT HERE
--
-- No delivery state: sending/sent/failed, the machine report_runs (0008) uses
-- and generated_activity_runs (0025) inherited, is for work that leaves the
-- building and can fail. Nothing here is delivered anywhere — a row IS the
-- notification, and it is read by a poll from a client that is signed in. If
-- this ever grows email or push, that state belongs on the delivery attempt and
-- not on this row.
--
-- No cleared_at either, and that is the deliberate part. "Clear all" deletes.
-- Rows are only ever written by live events, so nothing can resurrect one, and
-- a soft delete would buy nothing except an AND on every read and a table that
-- grows forever.

CREATE TABLE notifications (
    id SERIAL PRIMARY KEY,

    -- Who is being told, and who did the thing. Two references to the same
    -- table, and they are always different people: every insert filters
    -- actor_id out of the recipients, because being told about your own
    -- applause is noise.
    --
    -- Both cascade, following 0024's reasoning exactly. Removing an account
    -- takes the notifications it was sent AND the ones it caused — the second
    -- matters more here, because a notification whose actor is gone cannot be
    -- rendered at all: every row is drawn as "<somebody> did <something>", and
    -- the somebody is this column.
    user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    actor_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- 'reaction' | 'comment' | 'reply' | 'joined'.
    --
    -- Not constrained by a CHECK, and for a different reason than 0024 gave
    -- about emoji. That set needed validating because it arrives from a client;
    -- this one never does. A notification cannot be raised over the API at all
    -- — every kind here is written as a literal by one of the fan-out queries
    -- in notifications.sql, so the set of values this column can hold is
    -- already closed by the only code that writes it, and a CHECK would be a
    -- second copy of that list to keep in step across a deploy that adds one.
    kind TEXT NOT NULL,

    -- The subject, where there is one. All three are nullable because 'joined'
    -- has no session and no comment, and only 'reaction' has an emoji.
    --
    -- comment_id cascades, and that is how a deleted comment takes its
    -- notification with it — no handler has to remember. A reaction cannot do
    -- the same: 0024 gave session_reactions a composite primary key and no
    -- surrogate id, so there is nothing to reference and nothing to cascade
    -- from. Withdrawing applause deletes the matching row explicitly instead,
    -- matched on (user_id, actor_id, session_id, emoji). That asymmetry is the
    -- price of the composite key, and it is cheaper than adding a surrogate id
    -- to a table whose primary key is load-bearing idempotency.
    session_id INTEGER REFERENCES sessions(id) ON DELETE CASCADE,
    comment_id INTEGER REFERENCES session_comments(id) ON DELETE CASCADE,
    emoji      TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- NULL until read. A timestamp rather than a boolean because it costs the
    -- same and answers "when", which a boolean cannot be asked later.
    read_at TIMESTAMPTZ
);

-- The panel's only read: one lifter's notifications, newest first. Covers the
-- filter and both sort columns, so the list is an index scan with a LIMIT and
-- never a sort of everything the account has ever been sent.
--
-- id descends alongside created_at to break ties. The generator writes a day's
-- worth of reactions inside one transaction (see generateRecognition), so rows
-- sharing a created_at to the microsecond are ordinary here rather than
-- theoretical, and without the tiebreak the panel would reshuffle them between
-- polls.
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC, id DESC);

-- The badge's only read, asked on every poll by every signed-in client and
-- answered far more often than the list is opened. Partial, because the count
-- only ever asks about unread rows: the index holds the unread ones alone and
-- stays small on an install where most notifications have been read, which is
-- every install after its first week.
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;

-- ---- backfill ----
--
-- An install that has been social for a while already has applause and
-- conversation, and every install where generated activity has run has a lot of
-- it. Without this the feature ships as an empty panel on a populated install,
-- which reads as broken rather than as new.
--
-- Backfilled rows keep the ORIGINAL event's created_at, not now(), so the panel
-- opens in the order things actually happened. They land unread, so the first
-- open after this deploy shows a large count. That is correct — none of it has
-- been seen — and "mark all read" is one tap.
--
-- 'joined' is NOT backfilled. Every existing account would owe a row to every
-- other existing account, which is a square number of notifications announcing
-- news that is months old on the day it arrives.

INSERT INTO notifications (user_id, actor_id, kind, session_id, emoji, created_at)
SELECT s.user_id, r.user_id, 'reaction', r.session_id, r.emoji, r.created_at
FROM session_reactions r
JOIN sessions s ON s.id = r.session_id
-- Sessions predating 0005 have no owner, so there is nobody to tell.
WHERE s.user_id IS NOT NULL
  AND s.user_id <> r.user_id;

-- Comments fan out to the session's owner and to everyone already talking on
-- it, which is the same rule CreateCommentNotifications applies on every live
-- comment — including the part that keeps an owner who also commented from
-- being told twice about one comment.
--
-- The one thing this does that the live query does not need to: `prior` is
-- restricted to comments that came BEFORE this one. Live, that is true for
-- free, because the fan-out runs the moment the comment is inserted and every
-- other comment on the session is therefore older. Replaying history without
-- the restriction would tell the first commenter on a thread about a reply that
-- arrived before they had said anything.
INSERT INTO notifications (user_id, actor_id, kind, session_id, comment_id, created_at)
SELECT DISTINCT ON (c.id, recipient.user_id)
       recipient.user_id,
       c.user_id,
       recipient.kind,
       c.session_id,
       c.id,
       c.created_at
FROM session_comments c
JOIN sessions s ON s.id = c.session_id
CROSS JOIN LATERAL (
    SELECT s.user_id AS user_id, 'comment' AS kind
    UNION ALL
    SELECT prior.user_id, 'reply'
    FROM session_comments prior
    WHERE prior.session_id = c.session_id
      AND prior.created_at < c.created_at
) AS recipient
WHERE recipient.user_id IS NOT NULL
  AND recipient.user_id <> c.user_id
-- 'comment' outranks 'reply' for the same recipient: if you own the session you
-- are told you were commented on, not that somebody replied near you. Spelled
-- as a CASE rather than leaning on 'comment' < 'reply' alphabetically, which is
-- true but is not the reason.
ORDER BY c.id,
         recipient.user_id,
         CASE recipient.kind WHEN 'comment' THEN 0 ELSE 1 END;

COMMIT;
