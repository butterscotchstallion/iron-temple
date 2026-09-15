BEGIN;

-- Lossless: the function holds no data, it only spells out an arithmetic every
-- caller could inline again. Dropping it breaks the three queries that call it
-- until they are rewritten with the CASE written out — which is what the schema
-- looked like before 0019 — so this is only safe alongside reverting the
-- application code that ships those queries. That is the normal shape of a
-- down migration here, not a special hazard of this one.
DROP FUNCTION e1rm_lb(NUMERIC, INTEGER);

COMMIT;
