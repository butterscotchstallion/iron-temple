-- Bookkeeping for the Racked recap emails.
--
-- One row per (user, period) that has been sent or attempted. The table is not
-- a log — it is the thing that decides whether an email goes out, and the
-- reason the reporter can be a plain ticker inside the API rather than a cron.
--
-- The reporter asks "which completed periods have no successful row here?"
-- rather than waking at midnight on the 1st and firing. A clock-driven job that
-- misses its instant has missed it; a question asked every hour is answered
-- correctly the moment the process comes back, so downtime delays a recap
-- instead of dropping it.
--
-- The UNIQUE constraint is what makes that safe with more than one replica.
-- Claiming is an INSERT ... ON CONFLICT DO UPDATE guarded on status, so exactly
-- one replica can move a row into 'sending'; the others get no row back and do
-- nothing. Correctness comes from the constraint, not from counting processes.
--
-- status is deliberately three states rather than a sent_at null check:
--   sending  claimed, in flight. A crash here leaves it stuck, so claimed_at
--            lets a later tick reclaim a row that has been sending too long.
--   sent     delivered to the relay. Terminal.
--   failed   the relay refused. Retried on the next tick, up to a cap, then
--            left alone so it can be found rather than retried forever.
CREATE TABLE report_runs (
    id           SERIAL PRIMARY KEY,
    user_id      INTEGER     NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    period_kind  TEXT        NOT NULL CHECK (period_kind IN ('month', 'year')),
    -- The period's first day, so a month is 2026-03-01 and a year 2026-01-01.
    period_start DATE        NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'sending'
                             CHECK (status IN ('sending', 'sent', 'failed')),
    attempts     INTEGER     NOT NULL DEFAULT 0,
    claimed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at      TIMESTAMPTZ,
    last_error   TEXT        NOT NULL DEFAULT '',
    UNIQUE (user_id, period_kind, period_start)
);

-- The reporter's only lookup is "is this period outstanding for anyone", so the
-- period leads the index; the UNIQUE constraint above already covers per-user.
CREATE INDEX report_runs_period_idx ON report_runs (period_kind, period_start);
