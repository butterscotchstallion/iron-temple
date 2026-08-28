BEGIN;

-- Madcow 5x5, as Madcow rather than in name only.
--
-- 0012 seeded it as a two-day A/B of straight sets running on the linear engine,
-- which shares a name with the program a lifter picked and nothing else. Real
-- Madcow is a three-day week of RAMPING sets that progresses WEEKLY: a volume
-- day that climbs to a top set, a lighter day, and an intensity day with a heavy
-- triple and a backoff. Neither of those two properties could be expressed —
-- every set of a lift got one weight, and the engine advanced every session.
--
-- This migration adds the first (per-set prescriptions) and 0015's engine adds
-- the second.

-- ---------------------------------------------------------------------------
-- A second progression kind
-- ---------------------------------------------------------------------------
ALTER TABLE programs
    DROP CONSTRAINT programs_progression_kind_check;

ALTER TABLE programs
    ADD CONSTRAINT programs_progression_kind_check
    CHECK (progression_kind IN ('linear', 'madcow'));

-- ---------------------------------------------------------------------------
-- Per-set prescriptions
-- ---------------------------------------------------------------------------
--
-- One row per set of a prescribed lift, as a percentage of that lift's TOP SET
-- rather than as a weight. The percentages are the program; the weight they
-- resolve to is the lifter's, and moves every week.
--
-- Absence is meaningful and is the normal case: a lift with no rows here is a
-- uniform block of program_day_exercises.sets x .reps at one weight, which is
-- what every other program prescribes and what all of them keep doing. Only
-- Madcow populates this table, so nothing that exists today changes shape.
--
-- pct_of_top can exceed 100: Madcow's intensity day tops at 102.5% of the volume
-- day, which is the whole point of the day. The upper bound is a sanity rail
-- against a mistyped seed, not a rule about programming.
CREATE TABLE program_day_exercise_sets (
    program_day_exercise_id INTEGER NOT NULL
        REFERENCES program_day_exercises(id) ON DELETE CASCADE,
    set_number  INTEGER NOT NULL CHECK (set_number > 0),
    reps        INTEGER NOT NULL CHECK (reps > 0),
    pct_of_top  NUMERIC(5,2) NOT NULL CHECK (pct_of_top > 0 AND pct_of_top <= 200),
    PRIMARY KEY (program_day_exercise_id, set_number)
);

-- ---------------------------------------------------------------------------
-- Reseed Madcow
-- ---------------------------------------------------------------------------
--
-- 0012's two days go, and with them their prescriptions (ON DELETE CASCADE from
-- program_days). No session is touched: session_sets holds its own weights and
-- reps and references exercises, not prescriptions, so anything already logged
-- against the old shape stays exactly as it was logged. What changes is what the
-- program prescribes NEXT.
DELETE FROM program_days
WHERE program_id = (SELECT id FROM programs WHERE name = 'Madcow 5x5');

UPDATE programs
SET progression_kind = 'madcow',
    description = 'Three days a week of ramping fives: a volume day that climbs to a top set, a lighter day, then a heavy triple and a backoff. The top set goes up once a week, not once a session.'
WHERE name = 'Madcow 5x5';

INSERT INTO program_days (program_id, name, position)
SELECT p.id, v.name, v.position
FROM programs p
CROSS JOIN (VALUES
    ('Volume', 1),
    ('Light', 2),
    ('Intensity', 3)
) AS v(name, position)
WHERE p.name = 'Madcow 5x5';

-- The prescription rows. sets here is the number of set-plan rows below, and
-- reps is the rep count the block is named for — both are what a client with no
-- knowledge of set plans would render, and both stay truthful for one.
--
-- starting_weight_lb is the TOP set's starting weight, since that is what the
-- percentages are of. As everywhere else it is only ever consulted while the
-- lift has no history at all, and a lifter's own baseline displaces it.
INSERT INTO program_day_exercises
    (program_day_id, exercise_id, position, sets, reps, starting_weight_lb)
SELECT pd.id, e.id, v.position, v.sets, v.reps, v.starting_weight_lb
FROM (VALUES
    -- Volume: ramp to a top set of five. This is where squat, bench and row
    -- set the number every other day is a percentage of.
    ('Volume',    'Squat',          1, 5, 5, 45.0),
    ('Volume',    'Bench Press',    2, 5, 5, 45.0),
    ('Volume',    'Barbell Row',    3, 5, 5, 65.0),
    -- Light: the same ramp stopped one rung short for the squat, and the
    -- volume day proper for the press and the deadlift — they are trained once
    -- a week, so this is where THEIR top set lives.
    ('Light',     'Squat',          1, 4, 5, 45.0),
    ('Light',     'Overhead Press', 2, 4, 5, 45.0),
    ('Light',     'Deadlift',       3, 4, 5, 95.0),
    -- Intensity: ramp, a heavy triple above the volume day's top, then a
    -- backoff set of eight.
    ('Intensity', 'Squat',          1, 6, 5, 45.0),
    ('Intensity', 'Bench Press',    2, 6, 5, 45.0),
    ('Intensity', 'Barbell Row',    3, 6, 5, 65.0)
) AS v(day, exercise, position, sets, reps, starting_weight_lb)
JOIN program_days pd
  ON pd.name = v.day
 AND pd.program_id = (SELECT id FROM programs WHERE name = 'Madcow 5x5')
JOIN exercises e ON e.name = v.exercise AND e.created_by_user_id IS NULL;

-- The ramps.
--
-- Exactly one day per lift tops out at 100, and that day is the lift's
-- reference: the engine reads its history to decide the top set, and every other
-- day's weights are a percentage of it. Squat, bench and row reference the
-- volume day; the press and the deadlift reference the light day, because that
-- is the only day they are trained.
--
-- The intensity day's 102.5 is why pct_of_top is allowed above 100. It is a
-- heavier single effort than the volume day's top, taken for three rather than
-- five, and the 75 that follows it is the backoff.
INSERT INTO program_day_exercise_sets (program_day_exercise_id, set_number, reps, pct_of_top)
SELECT pde.id, v.set_number, v.reps, v.pct_of_top
FROM (VALUES
    -- Volume day: 5x5 ramping to the top set.
    ('Volume',    'Squat',          1, 5,  50.0), ('Volume',    'Squat',          2, 5,  62.5),
    ('Volume',    'Squat',          3, 5,  75.0), ('Volume',    'Squat',          4, 5,  87.5),
    ('Volume',    'Squat',          5, 5, 100.0),
    ('Volume',    'Bench Press',    1, 5,  50.0), ('Volume',    'Bench Press',    2, 5,  62.5),
    ('Volume',    'Bench Press',    3, 5,  75.0), ('Volume',    'Bench Press',    4, 5,  87.5),
    ('Volume',    'Bench Press',    5, 5, 100.0),
    ('Volume',    'Barbell Row',    1, 5,  50.0), ('Volume',    'Barbell Row',    2, 5,  62.5),
    ('Volume',    'Barbell Row',    3, 5,  75.0), ('Volume',    'Barbell Row',    4, 5,  87.5),
    ('Volume',    'Barbell Row',    5, 5, 100.0),

    -- Light day: the squat stops at 87.5 — the recovery day that keeps the
    -- pattern without the fatigue.
    ('Light',     'Squat',          1, 5,  50.0), ('Light',     'Squat',          2, 5,  62.5),
    ('Light',     'Squat',          3, 5,  75.0), ('Light',     'Squat',          4, 5,  87.5),
    -- The press and the deadlift ramp to their own top set here.
    ('Light',     'Overhead Press', 1, 5,  50.0), ('Light',     'Overhead Press', 2, 5,  70.0),
    ('Light',     'Overhead Press', 3, 5,  85.0), ('Light',     'Overhead Press', 4, 5, 100.0),
    ('Light',     'Deadlift',       1, 5,  50.0), ('Light',     'Deadlift',       2, 5,  70.0),
    ('Light',     'Deadlift',       3, 5,  85.0), ('Light',     'Deadlift',       4, 5, 100.0),

    -- Intensity day: ramp, a triple above the volume top, then a backoff eight.
    ('Intensity', 'Squat',          1, 5,  50.0), ('Intensity', 'Squat',          2, 5,  62.5),
    ('Intensity', 'Squat',          3, 5,  75.0), ('Intensity', 'Squat',          4, 5,  87.5),
    ('Intensity', 'Squat',          5, 3, 102.5), ('Intensity', 'Squat',          6, 8,  75.0),
    ('Intensity', 'Bench Press',    1, 5,  50.0), ('Intensity', 'Bench Press',    2, 5,  62.5),
    ('Intensity', 'Bench Press',    3, 5,  75.0), ('Intensity', 'Bench Press',    4, 5,  87.5),
    ('Intensity', 'Bench Press',    5, 3, 102.5), ('Intensity', 'Bench Press',    6, 8,  75.0),
    ('Intensity', 'Barbell Row',    1, 5,  50.0), ('Intensity', 'Barbell Row',    2, 5,  62.5),
    ('Intensity', 'Barbell Row',    3, 5,  75.0), ('Intensity', 'Barbell Row',    4, 5,  87.5),
    ('Intensity', 'Barbell Row',    5, 3, 102.5), ('Intensity', 'Barbell Row',    6, 8,  75.0)
) AS v(day, exercise, set_number, reps, pct_of_top)
JOIN program_days pd
  ON pd.name = v.day
 AND pd.program_id = (SELECT id FROM programs WHERE name = 'Madcow 5x5')
JOIN exercises e ON e.name = v.exercise AND e.created_by_user_id IS NULL
JOIN program_day_exercises pde
  ON pde.program_day_id = pd.id AND pde.exercise_id = e.id;

COMMIT;
