BEGIN;

-- A legs-and-glutes program built from resistance bands and free weights.
--
-- The eighth seeded program, and the first that is not from the StrongLifts
-- family. Two days — squat-led and hinge-led — in the shape 0002 established
-- and 0018 last reused: a program, its days, and a uniform block of sets x reps
-- at one weight per lift. No per-set prescriptions, so progression_kind stays
-- at its 'linear' default and program_day_exercise_sets gains nothing.
--
-- What is new is the band work, and it needs two things the catalogue did not
-- have: an equipment kind to call itself, and an engine that leaves it alone.
-- Both are below.

-- ---------------------------------------------------------------------------
-- 1. The catalogue learns about bands.
-- ---------------------------------------------------------------------------
--
-- 0009 constrained equipment to six kinds and put 'other' at the end as the
-- fallback for anything the app does not model. A band would fit there, and
-- that is exactly the argument against it: 'other' is where a movement goes
-- when nobody has decided what it is, and this install owns five bands. They
-- are equipment, the same way the bar and the plates 0013 recorded are, and a
-- lifter filtering the library for band work should find it rather than find
-- "Other".
--
-- Dropped and re-added rather than altered: Postgres has no ALTER CONSTRAINT
-- for a CHECK's expression. 0017 did the same to exercises_rest_seconds_check
-- and for the same reason, including keeping the name Postgres assigned the
-- inline CHECK, so the constraint holds one identity across both migrations.
--
-- 'band' goes before 'other' so the list still ends with its fallback. The
-- order is cosmetic to Postgres and load-bearing to the reader.
ALTER TABLE exercises
    DROP CONSTRAINT exercises_equipment_check;
ALTER TABLE exercises
    ADD CONSTRAINT exercises_equipment_check
        CHECK (equipment IN ('barbell', 'dumbbell', 'machine', 'cable',
                             'bodyweight', 'band', 'other'));

-- ---------------------------------------------------------------------------
-- 2. The movements this program needs that the catalogue lacks.
-- ---------------------------------------------------------------------------
--
-- is_accessory is false on all five: 0009's distinction is between what a
-- program puts on the bar and what a lifter bolts on afterwards, and a program
-- prescribes every one of these. Same call 0018 made when it promoted the
-- dumbbell press.
--
-- rest_seconds follows 0011's two surviving tiers as 0017's cap left them: 180
-- for a compound under real load, 90 for lighter and more local work. The hip
-- thrust is the first; banded walks, bridges and kickbacks are the second.
-- Both are well inside the 30..180 rail 0017 put in the schema.
--
-- muscle_group is 'legs' for all five. The catalogue has no 'glutes' group and
-- does not need one — 0009 chose seven groups that map to how a library is
-- browsed, not to the muscles a movement recruits, and a lifter looking for
-- hip thrusts looks under legs.
INSERT INTO exercises (name, muscle_group, equipment, is_accessory, rest_seconds)
VALUES
    ('Barbell Hip Thrust',    'legs', 'barbell', false, 180),
    ('Banded Lateral Walk',   'legs', 'band',    false,  90),
    ('Banded Hip Abduction',  'legs', 'band',    false,  90),
    ('Banded Glute Bridge',   'legs', 'band',    false,  90),
    ('Banded Kickback',       'legs', 'band',    false,  90)
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3. Promote the catalogue movements this program prescribes.
-- ---------------------------------------------------------------------------
--
-- Five lifts 0009 seeded as accessories are prescribed work now, so they stop
-- being accessories — the same promotion 0018 applied to the dumbbell press,
-- and for the same reason: the flag says what the app does with a movement, not
-- how hard it is.
--
-- rest_seconds is deliberately untouched, which is also 0018's precedent ("the
-- promotion costs it nothing"). Three of these already rest 180 by way of
-- 0011's compound and squat/deadlift tiers; the Bulgarian split squat and the
-- back extension rest 90, and 0011 said why in as many words — the split squat
-- is "in the family by name but not by load". Being prescribed does not change
-- what it loads, so it does not change what it rests.
--
-- Scoped to the shared catalogue. Without the created_by_user_id filter this
-- would reach into a lifter's own same-named movement, which 0009's per-owner
-- uniqueness permits, and reclassify something no program prescribes.
UPDATE exercises
   SET is_accessory = false
 WHERE name IN (
    'Front Squat',
    'Bulgarian Split Squat',
    'Romanian Deadlift',
    'Walking Lunge',
    'Back Extension'
 )
   AND created_by_user_id IS NULL;

-- ---------------------------------------------------------------------------
-- 4. The program.
-- ---------------------------------------------------------------------------
--
-- The description carries the band prescriptions, because nothing else can.
-- `exercises` has no notes column and program_day_exercises has no free text,
-- so which band to pull for which movement has exactly one place to live. That
-- is a real limit and not a happy one: the app cannot check the prose against
-- what the lifter owns, and cannot print "15-35 band" on the set card. Making
-- it data means a user_bands table beside user_plates, which is a gym-setup
-- feature rather than a program, and is deliberately not this migration.
INSERT INTO programs (name, description) VALUES
    ('Glutes & Legs (Bands and Free Weights)',
     'Two days for glutes and legs: squat-led, then hinge-led. Band work is ' ||
     'prescribed by band rather than by weight, so it logs at 0 lb and stays ' ||
     'there — lateral walks and hip abduction on the 15-35, the glute bridge ' ||
     'on the 30-60, kickbacks on the 5-15. Move up a band when the reps stop ' ||
     'being hard. The loaded lifts progress as everything else here does.')
ON CONFLICT (name) DO NOTHING;

INSERT INTO program_days (program_id, name, position)
SELECT p.id, d.name, d.position
FROM (VALUES
    ('Glutes & Legs (Bands and Free Weights)', 'Workout A', 1),
    ('Glutes & Legs (Bands and Free Weights)', 'Workout B', 2)
) AS d(program, name, position)
JOIN programs p ON p.name = d.program
ON CONFLICT (program_id, name) DO NOTHING;

-- Same shape as 0002, 0007 and 0018: (program, day, exercise, position, sets,
-- reps, starting_weight_lb).
--
-- THE BAND LIFTS START AT 0 AND NEVER MOVE. That is not a placeholder — it is
-- the prescription. A band carries no poundage, so there is no honest number to
-- put here, and 0 is what this app has always meant by "loaded with nothing":
-- bodyweight assistance logs at 0, warmup.ts emits no warm-ups at 0, and
-- PlateBar draws nothing for equipment that is not a barbell. Until this
-- migration a prescribed lift at 0 would have crept — NextPlan advanced any
-- successful session by the ladder's increment, so a banded lateral walk that
-- went well came back at 5 lb, then 10. The guard that stops it ships with this
-- program; see the zero-weight branch in progression.NextPlan. A lifter who
-- does decide to hang a plate off one of these logs a load, and from then on it
-- progresses like anything else — entering a weight is what opts a lift in.
--
-- The barbell weights assume a 45 lb bar, which is the convention every seeded
-- program follows and which 0013 spelled out: 45 is what a bar weighs when you
-- know nothing else about it. This install's bar is 80, so Front Squat at 45
-- is not merely light here, it is unloadable. The escape hatch is the one 0013
-- built and 0018 pointed at — a per-lifter starting weight at /me/baselines —
-- rather than seeding one gym's bar into shared data.
--
-- The dumbbell weights are the PAIR, as every weight in this app is the whole
-- load (see 0018). 30 lb is a pair of 15s, which a rack stepping 5 lb a bell
-- can build. Both dumbbell movements here hold one bell per hand, so the "15 lb
-- per hand" the set card prints is true of them; a goblet squat would have made
-- it a lie, which is why this program does not prescribe one.
INSERT INTO program_day_exercises
    (program_day_id, exercise_id, position, sets, reps, starting_weight_lb)
SELECT pd.id, e.id, v.position, v.sets, v.reps, v.starting_weight_lb
FROM (VALUES
    -- Squat-led. Bands first to open the hips, bands last to finish them.
    ('Glutes & Legs (Bands and Free Weights)','Workout A','Banded Lateral Walk',  1, 3, 15,  0.0),
    ('Glutes & Legs (Bands and Free Weights)','Workout A','Front Squat',          2, 3,  8, 45.0),
    ('Glutes & Legs (Bands and Free Weights)','Workout A','Barbell Hip Thrust',   3, 3,  8, 95.0),
    ('Glutes & Legs (Bands and Free Weights)','Workout A','Bulgarian Split Squat',4, 3,  8, 30.0),
    ('Glutes & Legs (Bands and Free Weights)','Workout A','Banded Hip Abduction', 5, 3, 20,  0.0),
    -- Hinge-led. The back extension is bodyweight, and takes the same zero
    -- guard the band work does for the same reason.
    ('Glutes & Legs (Bands and Free Weights)','Workout B','Banded Glute Bridge',  1, 3, 15,  0.0),
    ('Glutes & Legs (Bands and Free Weights)','Workout B','Romanian Deadlift',    2, 3,  8, 95.0),
    ('Glutes & Legs (Bands and Free Weights)','Workout B','Walking Lunge',        3, 3, 10, 30.0),
    ('Glutes & Legs (Bands and Free Weights)','Workout B','Back Extension',       4, 3, 12,  0.0),
    ('Glutes & Legs (Bands and Free Weights)','Workout B','Banded Kickback',      5, 3, 15,  0.0)
) AS v(program, day, exercise, position, sets, reps, starting_weight_lb)
JOIN programs p      ON p.name = v.program
JOIN program_days pd ON pd.program_id = p.id AND pd.name = v.day
-- Matched against the shared catalogue only, for the reason 0018 gives: without
-- the filter a lifter's own 'Walking Lunge' could join here too and seed the
-- day twice.
JOIN exercises e     ON e.name = v.exercise AND e.created_by_user_id IS NULL
ON CONFLICT (program_day_id, exercise_id) DO NOTHING;

COMMIT;
