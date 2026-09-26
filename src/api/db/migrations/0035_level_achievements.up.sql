-- The second kind of achievement: the rung reached for training enough of it.
--
-- 0032 built this table for the crown and said in as many words that it was meant
-- to hold more than one kind. This is the one it was waiting for, and it behaves
-- differently in the way that file predicted: a crown is a STANDING and is lost the
-- moment somebody else leads a board, where reaching Level 10 is a CROSSING and
-- cannot be un-reached.
--
-- WHY A DERIVED FIGURE IS ALLOWED AN ACHIEVEMENT HERE
--
-- The AchievementKind description used to refuse this outright: personal records and
-- lifetime milestones are derived at read time, and restating them as achievements
-- would give the install two answers about one history. A level is derived too — it
-- is a count of qualifying sessions, see db/queries/levels.sql — so that sentence
-- covered it and had to be rewritten rather than quietly ignored.
--
-- The line it now draws is between a standing and a crossing, and the difference is
-- whether a second copy can CONTRADICT the first. "Your best ever squat" is a
-- question the sessions answer, and a stored answer beside them can disagree.
-- "This lifter passed Level 10 on the 14th of March" is an event with a date; no
-- later edit makes it untrue, because it is not a restatement of the current level
-- at all. The level itself stays derived and unstored. What is written here is only
-- that a threshold was once passed.
--
-- THE CONSEQUENCE, STATED PLAINLY
--
-- A lifter who deletes sessions can drop below a rung they hold. /levels will say
-- Level 9 and their profile will say they reached Level 10, and BOTH ARE RIGHT —
-- one is a standing, the other is history. That is the first place in this schema
-- where two surfaces disagree on purpose, and it is worth knowing it was chosen
-- rather than overlooked. The alternative was closing the reign when the level
-- falls, which makes "reached Level 10" mean "is at least Level 10" and needs the
-- crown's reign-closing machinery for a thing that is supposed to be permanent.
--
-- Nothing ever closes a level reign. CloseAchievementReignsExcept is keyed on the
-- slug alone and has no notion of a permanent award, so the rail is that
-- refreshLevelAwards never calls it — see the note there.

BEGIN;

-- Which level a row is the rung for. NULL for every kind that is not a level's,
-- exactly as `metric` is NULL for every kind that is not a board's — and left
-- unconstrained for that column's reason: nothing arrives here from a client, the
-- set is closed by the migration that seeds it, and a CHECK naming `kind` would be
-- a second copy of the rule to keep in step.
--
-- In the table rather than parsed back out of the slug, and rather than held in Go
-- beside the reconciler. A slug is an opaque identifier everywhere else in this
-- schema and reading an integer out of one would make 'level-10' load-bearing
-- text; a copy in Go would be a second place the ladder is written down.
ALTER TABLE achievements
    ADD COLUMN level_threshold INTEGER;

-- The ladder. Four rungs across a lifter's first three years, and the spacing is
-- the point: at 100 XP a session and a curve where level L costs (L-1)*100, these
-- land at 10, 45, 190 and 435 sessions — about three weeks, three months, fifteen
-- months and just under three years at three sessions a week.
--
-- sort_order continues past the five crowns in tens rather than at 6, so a sixth
-- board can be seeded between them later without renumbering these.
INSERT INTO achievements (slug, kind, metric, level_threshold, label, description, sort_order) VALUES
    ('level-5', 'level', NULL, 5,
     'Initiate',
     'Reached Level 5 — ten sessions in, and past the point where a new lifter stops being new.',
     10),
    ('level-10', 'level', NULL, 10,
     'Journeyman',
     'Reached Level 10 — forty-five sessions, which is a training habit rather than a good month.',
     20),
    ('level-20', 'level', NULL, 20,
     'Veteran',
     'Reached Level 20 — a hundred and ninety sessions, better than a year of showing up.',
     30),
    ('level-30', 'level', NULL, 30,
     'Master of the Temple',
     'Reached Level 30 — four hundred and thirty-five sessions. Years of them.',
     40);

-- THE REIGNS ARE BACKFILLED, and that is the opposite of what 0032 did.
--
-- 0032 refused to backfill because who was top last month is unknowable: the
-- standings are computed on demand and the database has no record of them, so
-- inventing a held_from would have been fiction. Here it is not. The sessions are
-- right there, so who has passed Level 10 is a fact this migration can read, and
-- 0032's own standard allows the rest — it says stamping held_from = now() for
-- today's leaders "would be honest", and that is exactly what this does.
--
-- Doing it here is also what makes the first pass SILENT, structurally rather than
-- by a flag somebody has to remember. The reconciler announces every reign it
-- opens, so without this the first sweep after deploy would tell every lifter, and
-- every one of their followers, about rungs they passed months ago. 0026 refused to
-- backfill 'joined' notifications for precisely that reason. After this runs, the
-- only reigns left for the reconciler to open are genuine crossings.
--
-- The condition is the curve's session-cost form: reaching level L costs
-- L*(L-1)/2 sessions, because level L costs (L-1)*100 XP and a session is worth
-- 100. Integer arithmetic, no square root, and no need to derive a level at all —
-- comparing session counts against a threshold's cost answers the same question.
-- internal/levels still owns the curve; this is a one-off restatement of it and
-- says so.
--
-- The qualifying rule is levels.sql's, copied a third time because a generated
-- column cannot call now(). That file lists the copies; this one is historical and
-- runs once, so it is deliberately NOT added to that list — a migration that has
-- already run cannot be kept in step with anything.
--
-- On a fresh install this inserts nothing: no users, no sessions. migrate_test's
-- assertion that the ledger ships empty therefore still holds, and it should — it
-- is asserting what a new install looks like, which this does not change.
INSERT INTO lifter_achievements (user_id, achievement_slug)
SELECT u.id, a.slug
FROM users u
         JOIN achievements a ON a.kind = 'level'
WHERE (
          SELECT COUNT(*)
          FROM sessions s
          WHERE s.user_id = u.id
            AND (s.finished_at IS NOT NULL
              OR s.created_at < now() - INTERVAL '12 hours')
            AND EXISTS (SELECT 1
                        FROM session_sets ss
                        WHERE ss.session_id = s.id
                          AND ss.actual_reps > 0)
      ) >= a.level_threshold * (a.level_threshold - 1) / 2;

COMMIT;
