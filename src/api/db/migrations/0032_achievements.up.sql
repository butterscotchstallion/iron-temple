BEGIN;

-- Achievements, and the first kind of them: the crown for leading a leaderboard
-- board.
--
-- The leaderboard (see internal/api/leaderboard.go) already decides who leads
-- each of its five boards, and until now that was the only place anybody could
-- find out. A crown that follows a lifter around the site needs the same answer
-- somewhere every surface can afford to read.
--
-- WHY A TABLE AND NOT A QUERY
--
-- Because the query is the most expensive one in the app. /leaderboard is N
-- lifters times eight reads, run sequentially on purpose so a leaderboard cannot
-- starve the request that is actually logging a set. Deriving the crown on every
-- render of every name would put that behind the feed, the roster, the
-- notification panel and the header — it is not a question that can be asked per
-- page view, let alone per row.
--
-- So the standings are reconciled on the hourly sweeper and written here, and
-- the site-wide read becomes a single partial-index scan. The cost of that is
-- staleness: a crown can be up to an hour behind the leaderboard page, which is
-- live. That is the trade, and it is the right way round — the page where you go
-- to check the standings tells the truth immediately, and the ornament beside a
-- name catches up.
--
-- WHY TWO TABLES
--
-- achievements is a catalogue: what can be earned. lifter_achievements is the
-- ledger: who earned it and when. Splitting them means the label and the prose
-- live in one row rather than being copied onto every award, and a reword is an
-- UPDATE rather than a backfill.
--
-- The catalogue is seeded here rather than being inferred from the code that
-- writes it, which is the opposite of what 0026 did for notifications.kind. The
-- reason is that these rows are READ as content — a client draws the label and
-- the description — where a notification kind is only ever switched on. Content
-- belongs in a row; a closed set of branches does not.

CREATE TABLE achievements (
    -- Stable, human-readable, and the thing the client keys its icons off. A
    -- serial id would buy nothing: there is no user-entered achievement, the set
    -- changes only by migration, and a slug survives a dump/restore into a fresh
    -- install with the notification rows that reference it still meaning
    -- something.
    slug TEXT PRIMARY KEY,

    -- 'crown' is the only kind today. The column exists because this table is
    -- meant to hold more than one, and because kind is what a client switches on
    -- to decide how to draw an entry.
    --
    -- NOT constrained by a CHECK, for 0026's reason: nothing arrives here from a
    -- client, the set is closed by the migration that seeds it, and a CHECK would
    -- be a second copy of the list to keep in step.
    kind TEXT NOT NULL,

    -- Which leaderboard board this crown is for, matching the `metric` strings in
    -- leaderboard.go. NULL for any future kind that is not tied to a board.
    --
    -- This is the join back to the code that computes the standings, and it is a
    -- string rather than an enum so that adding a board is a seed row here and a
    -- case there, not a type change.
    metric TEXT,

    -- What a lifter is shown. Held here rather than in the client so that the
    -- five boards and the five crowns cannot drift into describing themselves
    -- differently.
    label       TEXT NOT NULL,
    description TEXT NOT NULL,

    -- The order surfaces list these in, matching buildBoards. Volume is last in
    -- both, and for the reason leaderboard.go gives at length: it ranks lifters
    -- by bodyweight and training age as much as by effort, so it is available
    -- rather than led with.
    sort_order INTEGER NOT NULL
);

-- One row per REIGN, not per lifter and not per award.
--
-- A reign is a continuous stretch of holding something. That shape is what makes
-- "held the crown three times" answerable at all, and it is why the reconciler
-- leaves an unchanged reign alone rather than restamping it: if every hourly
-- pass closed and reopened the current holder's row, a month on top would read as
-- seven hundred separate reigns and the count would be meaningless.
--
-- held_until IS NULL means "right now". A timestamp rather than a boolean for
-- 0026's reason — it costs the same and answers "when", which a boolean cannot be
-- asked later.
CREATE TABLE lifter_achievements (
    id SERIAL PRIMARY KEY,

    -- Cascades: an account that is removed takes its reigns with it. There is no
    -- reading of a reign that does not name the lifter who held it, so a row
    -- whose user is gone could not be drawn.
    user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    achievement_slug TEXT NOT NULL REFERENCES achievements(slug) ON DELETE CASCADE,

    held_from  TIMESTAMPTZ NOT NULL DEFAULT now(),
    held_until TIMESTAMPTZ,

    -- A reign cannot end before it began. Cheap, and it is the one way a bug in
    -- the reconciler's diff would show up as an error rather than as a profile
    -- quietly claiming a negative stretch.
    CONSTRAINT lifter_achievements_span_ck CHECK (held_until IS NULL OR held_until >= held_from)
);

-- At most one OPEN reign per lifter per achievement.
--
-- On the pair, deliberately, and not on achievement_slug alone: the leaderboard
-- gives tied figures the same rank (1, 2, 2, 4), so two lifters genuinely level
-- on a board are both first and both wear the crown. A unique index on the slug
-- would make that state unrepresentable and the reconciler would have to pick a
-- winner the standings refuse to pick.
--
-- What it does prevent is the reconciler opening a second reign for somebody who
-- already holds one, which is the failure the "leave it alone" rule above exists
-- to avoid — this index makes that rule enforced rather than merely intended.
CREATE UNIQUE INDEX lifter_achievements_current_idx
    ON lifter_achievements (achievement_slug, user_id)
    WHERE held_until IS NULL;

-- The site-wide read: everyone currently holding anything, asked by every signed-in
-- client so it can draw a crown beside a name.
--
-- Partial on the open reigns, the shape 0029 and 0030 both used. The ledger grows
-- forever by design — that is what keeping history means — but the number of open
-- reigns is bounded by five times the account count, so this index stays the size
-- of the install rather than the size of its history.
CREATE INDEX lifter_achievements_open_idx
    ON lifter_achievements (achievement_slug)
    WHERE held_until IS NULL;

-- The profile's read: one lifter's achievements, current and past, newest first.
CREATE INDEX lifter_achievements_user_idx
    ON lifter_achievements (user_id, held_from DESC);

-- ---- notifications ----
--
-- A crown changing hands is news, and 0026 anticipated exactly this: "a fifth
-- kind — a new lift record, somebody starting your program — is a row here and a
-- sentence in the client".
--
-- The subject column it needs is which board was won. session_id and comment_id
-- and emoji are all nullable for the same reason — not every kind has every
-- subject — and this is the fourth of those.
--
-- Note which way round the notification points. notifications.actor_id is NOT
-- NULL and every insert filters the actor out of the recipients, because being
-- told about your own applause is noise. So a crown notification tells EVERYBODY
-- ELSE that somebody took it; the holder's own recognition is the crown on their
-- name, which is the thing this whole migration exists to draw.
ALTER TABLE notifications
    ADD COLUMN achievement_slug TEXT REFERENCES achievements(slug) ON DELETE CASCADE;

-- No index for it. The panel's reads filter on user_id and group on
-- (kind, session_id) — 0031's index — and this column is only ever read back out
-- of a row that one of those already found.

-- ---- catalogue seed ----
--
-- One row per board in buildBoards order, with metric matching the constants in
-- leaderboard.go. The descriptions paraphrase each board's own note rather than
-- inventing a second account of what it measures.

INSERT INTO achievements (slug, kind, metric, label, description, sort_order) VALUES
    ('crown-sessions-per-week', 'crown', 'sessionsPerWeek',
     'Top of Sessions a week',
     'Trained more often than anyone else on the install this month, measured over the whole period rather than over the weeks they showed up.',
     1),
    ('crown-attendance', 'crown', 'attendance',
     'Top of Attendance',
     'Kept closest to the days their own program asked for this month. Lifters whose program carries no weekdays have no schedule to be measured against.',
     2),
    ('crown-improvement', 'crown', 'improvement',
     'Top of Most improved',
     'Made the biggest gain on a single lift this month, against their own earlier weight rather than against anybody else.',
     3),
    ('crown-streak', 'crown', 'streak',
     'Top of Week streak',
     'Held the longest run of consecutive weeks trained, ending at the last week they trained within the month.',
     4),
    ('crown-volume', 'crown', 'volume',
     'Top of Volume',
     'Moved more total weight than anyone else this month. Worth knowing and a poor contest: it ranks lifters by bodyweight and training age as much as by effort.',
     5);

-- No backfill of reigns, and that is deliberate.
--
-- A reign is a stretch of time somebody was top, and the database has no record
-- of who was top yesterday — the standings have only ever been computed on
-- demand. Inventing held_from = now() for today's leaders would be honest;
-- inventing anything earlier would be fiction. So the ledger starts empty and the
-- first sweeper pass, which runs within the hour and once at startup, opens the
-- first reigns. An install therefore shows no crowns for a few seconds after this
-- deploy and then shows the right ones.

COMMIT;
