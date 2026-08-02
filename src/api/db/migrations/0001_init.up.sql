BEGIN;

CREATE TABLE exercises (
    id         SERIAL PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE programs (
    id               SERIAL PRIMARY KEY,
    name             TEXT NOT NULL UNIQUE,
    description      TEXT NOT NULL DEFAULT '',
    -- Only linear progression today; kept as a column to admit others later.
    progression_kind TEXT NOT NULL DEFAULT 'linear'
                     CHECK (progression_kind IN ('linear')),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A day/variation within a program (e.g. Workout A / Workout B).
CREATE TABLE program_days (
    id         SERIAL PRIMARY KEY,
    program_id INTEGER NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    position   INTEGER NOT NULL CHECK (position > 0),
    UNIQUE (program_id, name),
    UNIQUE (program_id, position)
);

-- The prescription: what to lift on a given day.
CREATE TABLE program_day_exercises (
    id                 SERIAL PRIMARY KEY,
    program_day_id     INTEGER NOT NULL REFERENCES program_days(id) ON DELETE CASCADE,
    exercise_id        INTEGER NOT NULL REFERENCES exercises(id),
    position           INTEGER NOT NULL CHECK (position > 0),
    sets               INTEGER NOT NULL CHECK (sets > 0),
    reps               INTEGER NOT NULL CHECK (reps > 0),
    starting_weight_lb NUMERIC(6,2) NOT NULL CHECK (starting_weight_lb >= 0),
    UNIQUE (program_day_id, exercise_id),
    UNIQUE (program_day_id, position)
);

-- A performed instance of a program day.
CREATE TABLE sessions (
    id             SERIAL PRIMARY KEY,
    program_day_id INTEGER NOT NULL REFERENCES program_days(id),
    performed_on   DATE NOT NULL,
    notes          TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sessions_performed_on_idx ON sessions (performed_on);

-- The actual work logged, one row per set.
CREATE TABLE session_sets (
    id          SERIAL PRIMARY KEY,
    session_id  INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    exercise_id INTEGER NOT NULL REFERENCES exercises(id),
    set_number  INTEGER NOT NULL CHECK (set_number > 0),
    target_reps INTEGER NOT NULL CHECK (target_reps > 0),
    actual_reps INTEGER CHECK (actual_reps >= 0),          -- NULL until logged
    weight_lb   NUMERIC(6,2) NOT NULL CHECK (weight_lb >= 0),
    completed   BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (session_id, exercise_id, set_number)
);

CREATE INDEX session_sets_session_idx ON session_sets (session_id);

COMMIT;
