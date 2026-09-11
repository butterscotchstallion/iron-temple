BEGIN;

-- Three minutes is the longest rest the app prescribes.
--
-- 0011 gave rest three tiers: five minutes for the squat and deadlift families,
-- three for every other compound, ninety seconds for isolation work. The top
-- tier is now gone — the squat and the deadlift rest the same three minutes as
-- the bench press, leaving two tiers rather than three.
--
-- Written as a cap (rest_seconds > 180) rather than as 0011's name list. The
-- rule being applied is "nothing rests longer than three minutes", and a cap
-- says exactly that: it cannot miss a lift the list forgot, and it stays correct
-- if a later migration seeds a movement above the ceiling before this one runs.
UPDATE exercises SET rest_seconds = 180 WHERE rest_seconds > 180;

-- The CHECK ceiling comes down with the data, 900 -> 180.
--
-- 0011 called its bounds sanity rails rather than policy, and against a
-- five-minute top tier 900 was exactly that. With the cap, the rail and the
-- policy are the same number, and putting it in the schema is the point: the
-- ceiling holds for rows this migration never saw — a future seed, a restored
-- dump, a hand-written UPDATE — and not merely for the 53 it just rewrote.
--
-- Nothing legitimate is locked out today: rest_seconds is seeded and read, and
-- no endpoint writes it, so 180 is a ceiling on what the catalogue may declare
-- rather than on what a lifter may ask for. A per-lifter override would raise
-- this bound in its own migration, where the decision is visible.
--
-- Dropped and re-added rather than altered: Postgres has no ALTER CONSTRAINT for
-- a CHECK's expression. The name is the one 0011's inline CHECK got from
-- Postgres, so the constraint keeps a single identity across both migrations.
ALTER TABLE exercises
    DROP CONSTRAINT exercises_rest_seconds_check;
ALTER TABLE exercises
    ADD CONSTRAINT exercises_rest_seconds_check
        CHECK (rest_seconds BETWEEN 30 AND 180);

COMMIT;
