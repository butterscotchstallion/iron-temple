BEGIN;

-- Dropping these returns the app to a compiled-in 45 lb bar, an unbounded plate
-- set and the seeded starting weights. A lifter's recorded gym goes with them,
-- which is data they typed and cannot be recovered — but there is nowhere else
-- to keep it, and a down migration that preserved it would be inventing a
-- schema the up migration does not have.
DROP TABLE user_lift_baselines;
DROP TABLE user_plates;
DROP TABLE user_gym;

COMMIT;
