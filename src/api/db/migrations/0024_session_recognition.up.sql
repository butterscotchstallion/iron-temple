BEGIN;

-- Applause and conversation on a session, for an install shared by more than one
-- lifter.
--
-- 0005 made training per-user and every query since has been scoped to one
-- account. These are the first two tables in the schema whose whole purpose is
-- for one lifter's row to hang off another lifter's session — the session is the
-- subject, the user is the author, and they are deliberately different people in
-- the ordinary case.
--
-- Both cascade on BOTH foreign keys. Deleting a session takes its applause with
-- it, which is right: a reaction is about a workout and means nothing without
-- one. Deleting an account takes its reactions and comments, which is also
-- right, and is what makes the admin area's "remove an account" not leave
-- unattributable rows behind. Neither is training history — the one thing in this
-- database that cannot be recomputed — so neither needs the careful refusal that
-- the program down-migrations make about session_sets.

CREATE TABLE session_reactions (
    session_id INTEGER     NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    user_id    INTEGER     NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- The emoji itself rather than a code, so a row is readable in psql and the
    -- API needs no lookup table to render one.
    --
    -- Not constrained by a CHECK, and that is a decision rather than an
    -- omission. Which emoji this app offers is a product question that will
    -- change — four today, a fifth when somebody asks — and a schema migration
    -- per emoji is a poor trade for a set the API validates on every write
    -- anyway (see allowedReactions in internal/api). A CHECK would also have to
    -- be widened BEFORE a deploy that offers a new one and narrowed after a
    -- rollback, which is a two-step dance around a one-line constant.
    emoji      TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- The primary key is the idempotency. A lifter tapping 💪 twice is one
    -- reaction, and that is settled by the database rather than by a handler
    -- remembering to check first — which under concurrent taps it could not do
    -- correctly anyway. The API inserts with ON CONFLICT DO NOTHING and a second
    -- tap is a no-op rather than a 409, because a double-tap is not an error the
    -- lifter should be told about.
    --
    -- Column order matters for the index this creates: session_id leads, and
    -- every read is "the reactions on this session", so that index serves them
    -- without a second one.
    PRIMARY KEY (session_id, user_id, emoji)
);

-- Everything one lifter has reacted to, for the "did I already applaud this"
-- question asked per session. The PK above cannot serve it — its leading column
-- is the session — and without this that read is a scan once an install has any
-- history.
CREATE INDEX session_reactions_user_idx ON session_reactions (user_id);

CREATE TABLE session_comments (
    id         SERIAL      PRIMARY KEY,
    session_id INTEGER     NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    user_id    INTEGER     NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Length is bounded by the API (see maxCommentBody) rather than by the
    -- column. TEXT with a limit checked in one place beats VARCHAR(n) that has to
    -- agree with a constant in Go, and the check the API makes counts RUNES —
    -- so a comment of 200 emoji is accepted where a byte-counting column
    -- constraint would have refused it for being 800 bytes.
    body       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Comments are read a session at a time, oldest first, which is a conversation's
-- natural order. The index covers both the filter and the sort.
CREATE INDEX session_comments_session_idx ON session_comments (session_id, created_at);

COMMIT;
