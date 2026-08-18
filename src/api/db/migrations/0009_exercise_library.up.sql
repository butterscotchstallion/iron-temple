BEGIN;

-- The exercise library, and assistance work layered onto a program day.
--
-- 0007 seeded the Intermediate program with the description "Assistance work is
-- the lifter's choice" and then noted that assistance was deliberately unseeded,
-- because "it has no fixed lift, sets or reps, and the schema has nowhere to put
-- a prescription without them". This migration builds that missing half.
--
-- The design constraint that shapes everything below: PROGRAMS ARE NEVER EDITED.
-- programs, program_days and program_day_exercises stay exactly as seeded —
-- shared between every account, and canonical. A lifter's assistance work is an
-- additive per-user overlay in a separate table, never a mutation of the
-- prescription. That is what keeps the progression engine and the Racked recap
-- honest: ListLiftHistory reads the untouched prescription, so a squat
-- progresses identically whether or not the lifter also does curls, and a
-- period-over-period comparison of the main lifts compares the same lifts.

-- ---------------------------------------------------------------------------
-- 1. Exercises grow the metadata a browsable library needs, plus an owner.
-- ---------------------------------------------------------------------------

ALTER TABLE exercises
    ADD COLUMN muscle_group TEXT NOT NULL DEFAULT 'other'
        CHECK (muscle_group IN ('chest', 'back', 'legs', 'shoulders', 'arms', 'core', 'other')),
    ADD COLUMN equipment TEXT NOT NULL DEFAULT 'other'
        CHECK (equipment IN ('barbell', 'dumbbell', 'machine', 'cable', 'bodyweight', 'other')),
    -- false for the compound lifts the programs prescribe, true for accessory
    -- work. The distinction is what lets the library lead with the movements a
    -- lifter is actually shopping for when adding assistance.
    ADD COLUMN is_accessory BOOLEAN NOT NULL DEFAULT true,
    -- NULL means seeded: shared by everyone, and not deletable by anyone.
    -- Non-NULL means one lifter's own movement, visible only to them.
    --
    -- No ON DELETE clause on purpose. CASCADE would delete a lifter's custom
    -- exercises when their account goes, and session_sets.exercise_id has no
    -- cascade of its own — so the delete would either fail halfway or orphan
    -- logged history. There is no account-deletion path today; when there is,
    -- it has to decide what happens to that history deliberately rather than
    -- inherit an answer from this line.
    ADD COLUMN created_by_user_id INTEGER REFERENCES users(id);

-- Name uniqueness becomes per-owner. Two lifters may each have their own
-- "Copenhagen Plank"; neither may have two.
--
-- Case-insensitive, matching the users_username_lower_idx precedent in 0005: a
-- library that lists both "Face Pull" and "face pull" is a library with a bug in
-- it, and the API compares names case-insensitively when rejecting duplicates.
ALTER TABLE exercises DROP CONSTRAINT exercises_name_key;

CREATE UNIQUE INDEX exercises_name_shared_idx
    ON exercises (lower(name))
    WHERE created_by_user_id IS NULL;

CREATE UNIQUE INDEX exercises_name_owned_idx
    ON exercises (lower(name), created_by_user_id)
    WHERE created_by_user_id IS NOT NULL;

-- Every library read filters on the owner ("mine or everyone's").
CREATE INDEX exercises_owner_idx ON exercises (created_by_user_id);

-- ---------------------------------------------------------------------------
-- 2. Classify the lifts seeded by 0002 and 0007.
-- ---------------------------------------------------------------------------
--
-- These are the programs' prescribed work, so is_accessory is false: they are
-- what a program puts on the bar, not what a lifter bolts on afterwards.

UPDATE exercises e
SET muscle_group = v.muscle_group,
    equipment    = 'barbell',
    is_accessory = false
FROM (VALUES
    ('Squat',               'legs'),
    ('Bench Press',         'chest'),
    ('Barbell Row',         'back'),
    ('Overhead Press',      'shoulders'),
    ('Deadlift',            'back'),
    ('Pause Squat',         'legs'),
    ('Pause Bench Press',   'chest'),
    ('Pause Deadlift',      'back'),
    ('Incline Bench Press', 'chest'),
    ('Feet-Up Bench Press', 'chest')
) AS v(name, muscle_group)
WHERE e.name = v.name;

-- ---------------------------------------------------------------------------
-- 3. Seed the accessory catalogue.
-- ---------------------------------------------------------------------------
--
-- Broad enough that the common assistance choices are all present without
-- anyone having to type one in, and deliberately conventional in naming so the
-- UI's keyword-to-emoji mapping (src/ui/src/lib/exerciseIcon.ts) resolves them.
--
-- This cannot use ON CONFLICT (name) the way 0002 and 0007 did — that constraint
-- was just replaced by the two partial indexes above, and neither is inferrable
-- from a bare column list. NOT EXISTS says the same thing and reads plainly.

INSERT INTO exercises (name, muscle_group, equipment)
SELECT v.name, v.muscle_group, v.equipment
FROM (VALUES
    -- chest
    ('Dumbbell Bench Press',        'chest',     'dumbbell'),
    ('Dumbbell Incline Press',      'chest',     'dumbbell'),
    ('Dumbbell Fly',                'chest',     'dumbbell'),
    ('Cable Fly',                   'chest',     'cable'),
    ('Machine Chest Press',         'chest',     'machine'),
    ('Push-Up',                     'chest',     'bodyweight'),
    ('Dip',                         'chest',     'bodyweight'),
    -- back
    ('Pull-Up',                     'back',      'bodyweight'),
    ('Chin-Up',                     'back',      'bodyweight'),
    ('Lat Pulldown',                'back',      'cable'),
    ('Seated Cable Row',            'back',      'cable'),
    ('Dumbbell Row',                'back',      'dumbbell'),
    ('T-Bar Row',                   'back',      'barbell'),
    ('Face Pull',                   'back',      'cable'),
    ('Barbell Shrug',               'back',      'barbell'),
    ('Back Extension',              'back',      'bodyweight'),
    -- legs
    ('Romanian Deadlift',           'legs',      'barbell'),
    ('Front Squat',                 'legs',      'barbell'),
    ('Goblet Squat',                'legs',      'dumbbell'),
    ('Bulgarian Split Squat',       'legs',      'dumbbell'),
    ('Walking Lunge',               'legs',      'dumbbell'),
    ('Leg Press',                   'legs',      'machine'),
    ('Leg Curl',                    'legs',      'machine'),
    ('Leg Extension',               'legs',      'machine'),
    ('Standing Calf Raise',         'legs',      'machine'),
    -- shoulders
    ('Dumbbell Shoulder Press',     'shoulders', 'dumbbell'),
    ('Arnold Press',                'shoulders', 'dumbbell'),
    ('Lateral Raise',               'shoulders', 'dumbbell'),
    ('Rear Delt Fly',               'shoulders', 'dumbbell'),
    ('Upright Row',                 'shoulders', 'barbell'),
    -- arms
    ('Barbell Curl',                'arms',      'barbell'),
    ('Dumbbell Curl',               'arms',      'dumbbell'),
    ('Hammer Curl',                 'arms',      'dumbbell'),
    ('Preacher Curl',               'arms',      'barbell'),
    ('Triceps Pushdown',            'arms',      'cable'),
    ('Skull Crusher',               'arms',      'barbell'),
    ('Overhead Triceps Extension',  'arms',      'dumbbell'),
    ('Close-Grip Bench Press',      'arms',      'barbell'),
    -- core
    ('Plank',                       'core',      'bodyweight'),
    ('Hanging Leg Raise',           'core',      'bodyweight'),
    ('Ab Wheel Rollout',            'core',      'bodyweight'),
    ('Cable Crunch',                'core',      'cable'),
    ('Russian Twist',               'core',      'bodyweight')
) AS v(name, muscle_group, equipment)
WHERE NOT EXISTS (
    SELECT 1 FROM exercises e WHERE lower(e.name) = lower(v.name)
);

-- ---------------------------------------------------------------------------
-- 4. Assistance work: a lifter's additions to a shared program day.
-- ---------------------------------------------------------------------------
--
-- Keyed on (user_id, program_day_id) rather than living in
-- program_day_exercises, which is the whole point: the same Workout A is
-- squat/bench/row for everyone, and what one lifter bolts onto the end of it is
-- invisible to the next. Deleting a row deletes the plan, never the history —
-- sets already logged against the exercise stay in session_sets and keep
-- counting, which is why ListSessionSets orders with a fallback for sets whose
-- assistance row has since gone.
--
-- weight_lb has no progression engine behind it. The API prescribes the top
-- weight last logged for the lift and falls back to this column, so bodyweight
-- work needs no number entered (hence the 0 default) and a lifter who adds a
-- plate to their dip belt simply logs it, and finds it there next time.
CREATE TABLE program_day_assistance (
    id             SERIAL PRIMARY KEY,
    user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    program_day_id INTEGER NOT NULL REFERENCES program_days(id) ON DELETE CASCADE,
    exercise_id    INTEGER NOT NULL REFERENCES exercises(id) ON DELETE CASCADE,
    -- Ordering within the day's assistance block. Assigned max+1 on insert and
    -- read as (position, id), so it needs no uniqueness constraint of its own —
    -- there is no reordering UI to make two rows collide.
    position       INTEGER NOT NULL CHECK (position > 0),
    sets           INTEGER NOT NULL CHECK (sets > 0),
    reps           INTEGER NOT NULL CHECK (reps > 0),
    weight_lb      NUMERIC(6,2) NOT NULL DEFAULT 0 CHECK (weight_lb >= 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- One entry per lift per day: "3x8 dips and also 3x10 dips" is one entry
    -- with the sets edited, not two rows to reconcile.
    UNIQUE (user_id, program_day_id, exercise_id)
);

-- Every read is "this lifter's assistance for this day", including the join in
-- ListSessionSets that orders a session's sets.
CREATE INDEX program_day_assistance_day_idx
    ON program_day_assistance (user_id, program_day_id);

COMMIT;
