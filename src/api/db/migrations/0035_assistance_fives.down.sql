BEGIN;

-- Deliberately empty, and that is the honest answer rather than a missing one.
--
-- The up migration read rows and overwrote them; it did not add a structure that
-- can be taken away again. Which assistance rows carried 8–12 is not recorded
-- anywhere once they are nulled, so there is nothing to restore FROM.
--
-- The two ways to fake it are both worse than doing nothing:
--
--   - Put 8–12 back on every assistance row. That would range lifts that never
--     had one — every accessory added before the default flipped on, and every
--     one where a lifter unticked it on purpose — and silently overwrite their
--     reps with 8. A down migration that loses more data than the up did.
--   - Put it back on rows that still read `reps = 5`. Indistinguishable from a
--     lift somebody deliberately set to 5, which after this ships is most of
--     them.
--
-- So: rolling back to 0034 leaves assistance work on fives with no range. The
-- schema is correct for that version — 0014's columns and constraint are still
-- there, untouched by this pair — and any lift that wants a range can be given
-- one in the program editor, which is how it was always meant to be set.

SELECT 1;

COMMIT;
