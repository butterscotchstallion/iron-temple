BEGIN;

-- Bookkeeping for generating activity once a day, unattended.
--
-- 0024 gave the install applause and conversation; the generator that fills them
-- in was manual — press a button for history, or start a loop that dies with the
-- process. Neither keeps an install looking alive next week. These two tables are
-- what let it run on its own, and they are modelled on report_runs (0008) because
-- that table solved the same problem first and its reasoning holds here.

-- The schedule itself. One row, ever.
--
-- Persisted rather than held in memory, which is the whole point: the live loop's
-- flag is a field on the server and does not survive a restart, so "runs daily"
-- could not be built on it. An operator turns this on once and it stays on across
-- deploys.
--
-- A single-row settings table rather than a key/value store, because these are two
-- typed columns with an obvious shape and a KV table would turn `lifters` into a
-- string somebody has to parse. The CHECK on id is what makes "one row" a property
-- of the schema instead of a convention the application has to keep — an INSERT of
-- a second row fails rather than silently creating a schedule nobody reads.
CREATE TABLE generated_activity_schedule (
    id         INTEGER     PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    -- Off until somebody asks for it. A fresh install, and every existing one
    -- this migration runs against, generates nothing until the owner says so.
    enabled    BOOLEAN     NOT NULL DEFAULT false,
    -- How many of the roster to keep active. Bounded by the API against the
    -- roster's real size rather than by a CHECK here, so adding a persona is a
    -- constant rather than a migration.
    lifters    INTEGER     NOT NULL DEFAULT 4,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seeded so readers never have to handle "no row yet". Every query below can then
-- be a plain SELECT or UPDATE rather than an upsert, and the API has one less
-- empty case to get wrong.
INSERT INTO generated_activity_schedule (id) VALUES (1);

-- One row per day that has been generated.
--
-- This is not a log. It is the thing that DECIDES whether a day gets generated,
-- and it is the reason the scheduler can be a plain hourly ticker inside the API
-- rather than a cron entry. The scheduler asks "which recent days have no row
-- here?" instead of waking at midnight: a clock-driven job that misses its instant
-- has missed it, where a question asked every hour is answered correctly the
-- moment the process comes back. So downtime delays a day's activity rather than
-- losing it. Exactly report_runs' argument, and it is why the catch-up is bounded
-- in Go rather than unbounded here — see DueDays.
--
-- day is the PRIMARY KEY, and that is what makes claiming safe with more than one
-- replica. Claiming is an INSERT ... ON CONFLICT DO NOTHING that returns the row
-- it inserted: exactly one caller can create a given day, the losers get nothing
-- back and do nothing. Correctness comes from the constraint rather than from
-- counting processes.
--
-- No status column, unlike report_runs, and the difference is real rather than an
-- omission. That table needs sending/sent/failed because delivery is somebody
-- else's system and can fail halfway. Generating is local work in one transaction:
-- it either committed or it did not. A day whose generation fails has its claim
-- DELETED, so the next tick finds it outstanding again and retries — which is the
-- same recovery a 'failed' row buys, without a state machine to keep honest.
CREATE TABLE generated_activity_runs (
    day         DATE        PRIMARY KEY,
    -- Recorded so the admin screen can say what the last run actually did rather
    -- than only that one happened. Counted, not derived: the sessions a day
    -- produced cannot be recovered later, because nothing marks them.
    sessions    INTEGER     NOT NULL DEFAULT 0,
    reactions   INTEGER     NOT NULL DEFAULT 0,
    comments    INTEGER     NOT NULL DEFAULT 0,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;
