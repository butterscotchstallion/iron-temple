BEGIN;

-- A rep range for assistance work, and with it the only progression that runs on
-- accessories.
--
-- Until now nothing moved an accessory's weight. prescribe() carries forward
-- whatever was last logged for the lift, and that is all — the comment there
-- argues the case, and it is a good one: "A curl is not a squat: it does not
-- advance five pounds a session, and stalling on one is not a signal worth
-- deloading over."
--
-- This does not overturn that. It adds the progression accessories actually run
-- on, which is on REPS rather than on weight: pick a range, add reps inside it
-- week to week, and when every set reaches the top the weight goes up and the
-- reps reset to the bottom. No deload, ever — a stalled curl is still not a
-- signal.
--
-- Both columns are nullable and NULL is the default, so this is opt-in per lift
-- and every existing row keeps the carry-forward behaviour untouched.
ALTER TABLE program_day_assistance
    ADD COLUMN rep_min INTEGER CHECK (rep_min > 0),
    ADD COLUMN rep_max INTEGER CHECK (rep_max > 0);

-- Either both or neither, and a range that runs the right way. A half-configured
-- range has no sensible reading — "8 to null" is not a range, it is a mistake —
-- and the engine would have to invent a rule for it.
ALTER TABLE program_day_assistance
    ADD CONSTRAINT program_day_assistance_rep_range_ck
    CHECK (
        (rep_min IS NULL AND rep_max IS NULL)
        OR (rep_min IS NOT NULL AND rep_max IS NOT NULL AND rep_max >= rep_min)
    );

COMMIT;
