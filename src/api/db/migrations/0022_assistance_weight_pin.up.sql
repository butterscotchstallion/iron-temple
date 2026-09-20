BEGIN;

-- Let a lifter actually change an assistance weight.
--
-- Editing the weight on the program page has never done anything to a lift that
-- had been performed, and there was no way to tell from the screen. Two
-- separate things conspired. The engine's carry-forward displaces the stored
-- weight with the last one logged — deliberately, so a curl logged at 65 in a
-- session still in progress is not forgotten ten minutes later — and the
-- program page renders the PRESCRIBED weight rather than the stored one. So a
-- curl that had climbed to 50 read 50, an edit to 30 wrote 30 to a column
-- nothing consulted, and the row went on reading 50. The lifter's only working
-- lever was the +/- buttons inside a live session.
--
-- The fix has to keep the carry-forward, which is right, while letting an
-- explicit edit outrank it. Those are the same question asked twice — "what
-- weight does this lift start from" — and the answer is whichever the lifter
-- said more recently. So this records WHEN the weight was set, in the only unit
-- that can be compared against a performance without a clock.
--
-- A session id, not a timestamp. The obvious column here is `weight_set_at
-- TIMESTAMPTZ` compared against sessions.performed_on, and it is wrong in the
-- case that matters most: performed_on is a DATE, so a lifter who trains at 9am
-- and decides at 10am that 50 was too heavy writes a pin that is "after"
-- midnight and therefore after their own session — which reads correctly — but
-- then trains the next week at the pinned weight and finds the pin STILL newer
-- than the date it is compared to. The pin would never release and the lift
-- would never progress again. Ids are monotonic and exact where the date is
-- rounded, so the comparison stays right without widening sessions.performed_on
-- into a timestamp that nothing else wants.
--
-- The value is the most recent session this exercise was logged in at the
-- moment the weight was set, or 0 when it had never been logged. The pin is
-- spent as soon as that stops being the most recent session — which is to say,
-- the next time the lift is performed. That is what "your edit holds until you
-- next lift it" means, and it needs no clearing write and no background job:
-- the pin is not erased, it simply stops matching.
--
-- NULL means the weight has never been explicitly set, which is every row that
-- exists today. Those keep carrying forward exactly as they did, so this
-- migration changes no prescription until a lifter edits a weight on purpose.
--
-- No foreign key. This is a watermark, not a relationship: nothing here needs
-- the session to still exist, and ON DELETE CASCADE would be actively wrong —
-- deleting an old session must not silently re-arm a pin the lifter already
-- spent. Ids are never reused, so a dangling value compares false forever,
-- which is the behaviour wanted.
ALTER TABLE program_day_assistance
    ADD COLUMN weight_set_after_session_id INTEGER
        CHECK (weight_set_after_session_id >= 0);

COMMIT;
