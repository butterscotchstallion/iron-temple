BEGIN;

-- What the lifter can actually load, and where each lift starts.
--
-- Three tables for one idea: the numbers this app puts on the bar have to be
-- numbers this gym can build. Until now both halves were guesses baked into the
-- front-end bundle — BAR_LB was a module constant in plates.ts, PLATES_LB was an
-- unbounded 45-down-to-2.5 set, and the seeded starting weights assumed a 45 lb
-- bar that this install does not own. The result was a fresh account being told
-- to squat 45 lb on an 80 lb bar for its first seven sessions.
--
-- Deliberately NOT columns on `users`. Every query in users.sql lists its
-- columns explicitly and in table order — that is what makes sqlc return the
-- User model instead of a bespoke row struct per query, and users.sql says so.
-- Widening that table means hand-editing five generated queries and their scan
-- order, in a sandbox that cannot run sqlc to check the result. Separate tables
-- are additive, and gym setup is a different concern from identity anyway.

-- ---------------------------------------------------------------------------
-- The bar
-- ---------------------------------------------------------------------------
--
-- One row per lifter, created on demand. A missing row means the default, so
-- registration does not have to write one and no backfill is needed here; see
-- GetGymSetup, which COALESCEs rather than requiring the row to exist.
--
-- The DEFAULT is 45 and not 80. 80 is *this* install's bar, which is data and is
-- written as data below; 45 is what a bar weighs when you do not know anything
-- else about it, which is what a default is for.
CREATE TABLE user_gym (
    user_id       INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    bar_weight_lb NUMERIC(6,2) NOT NULL DEFAULT 45 CHECK (bar_weight_lb > 0),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- The plates
-- ---------------------------------------------------------------------------
--
-- pairs, not a count: plates are loaded symmetrically, so owning three 45s means
-- being able to load one per side and no more. Storing what you own in the unit
-- you can use it in keeps the loader from having to halve and round.
--
-- Unlike user_gym, absence here is meaningful rather than a fallback — a lifter
-- with no rows owns no plates and every prescription is bar-only. That is why
-- both this migration (below) and register() seed the standard set: "no rows"
-- has to mean what it says, so it cannot also be doing duty as "not configured".
CREATE TABLE user_plates (
    user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plate_lb NUMERIC(5,2) NOT NULL CHECK (plate_lb > 0),
    pairs    INTEGER NOT NULL CHECK (pairs > 0),
    PRIMARY KEY (user_id, plate_lb)
);

-- ---------------------------------------------------------------------------
-- Starting weights
-- ---------------------------------------------------------------------------
--
-- A per-lifter override for where a lift starts, consulted only when that lift
-- has no history at all. The seeded programs stay shared and untouched — the
-- same overlay pattern program_day_assistance uses, and for the same reason:
-- your starting squat should not move anyone else's Workout A.
--
-- Keyed by exercise rather than by program day, matching the grain the
-- progression engine now reads history at (see the scope note on ListLiftHistory
-- in db/queries/programs.sql). Where you start the squat is a fact about you,
-- not about which program sent you to it, so one baseline serves every program.
--
-- weight_lb >= 0 rather than > 0: an unloaded bar is a legitimate place to start
-- a press, and 0 means exactly that rather than "unset" — an unset baseline has
-- no row.
CREATE TABLE user_lift_baselines (
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    exercise_id INTEGER NOT NULL REFERENCES exercises(id) ON DELETE CASCADE,
    weight_lb   NUMERIC(6,2) NOT NULL CHECK (weight_lb >= 0),
    PRIMARY KEY (user_id, exercise_id)
);

-- ---------------------------------------------------------------------------
-- Seed the existing account(s)
-- ---------------------------------------------------------------------------
--
-- The standard home-gym set, two pairs of each. A guess, but a checkable one:
-- it is visible and editable on the profile screen the moment this ships, which
-- an unbounded assumption compiled into the bundle never was.
INSERT INTO user_plates (user_id, plate_lb, pairs)
SELECT u.id, v.plate_lb, v.pairs
FROM users u
CROSS JOIN (VALUES
    (45.0, 2),
    (35.0, 2),
    (25.0, 2),
    (10.0, 2),
    ( 5.0, 2),
    ( 2.5, 2)
) AS v(plate_lb, pairs)
ON CONFLICT DO NOTHING;

-- This install's actual bar, recorded as the data it always was. 6fe1301 set the
-- UI constant to 80 with the note "the actual bar is 80 lb"; this is that fact
-- moving out of the bundle and into the row it belongs in.
INSERT INTO user_gym (user_id, bar_weight_lb)
SELECT id, 80.0 FROM users
ON CONFLICT DO NOTHING;

COMMIT;
