BEGIN;

-- Who wants to hear about whom.
--
-- 0032 gave the install crowns, and announced every one of them to everybody:
-- CreateCrownNotifications fanned out to the whole roster minus the lifter who
-- took it. That was modelled on CreateJoinNotifications and it does not hold up.
-- On a box shared by a gym crew, "somebody you have never spoken to took the
-- Volume crown" is noise, and the one person it genuinely mattered to — the lifter
-- who earned it — was the only one not told.
--
-- So achievement notifications now go to the lifter themselves, plus anybody who
-- has chosen to follow them. This table is that choice.
--
-- THIS IS NOT A VISIBILITY SETTING, and the distinction is the whole argument for
-- it being allowed to exist. internal/api/lifters.go says at length that this app
-- has no per-user privacy filter, that admission to the install is the consent,
-- and that "anyone adding one is changing this premise rather than fixing an
-- omission". Nothing here changes that premise. Every profile, every session,
-- every board and every crown stays exactly as readable as it was; a follow
-- decides what gets PUSHED at you, not what you may go and look at. The roster
-- still lists everybody, and it always will.
--
-- Nor is it about volume. The comments in 0030 and 0031 have already argued that a
-- household install's notification count is small, and they are right — this would
-- be a poor optimisation. It is about RELEVANCE: a panel where every row concerns
-- somebody you chose is a panel worth opening.
--
-- WHY THERE IS NO BACKFILL
--
-- Because nothing is lost without one. A lifter with no follows still hears about
-- their own achievements, so the panel is never silent on the day this deploys and
-- there is no state to invent. Compare 0032, which refused to backfill reigns
-- because it would have had to fabricate a held_from; here the honest starting
-- state — "you follow nobody yet" — is also the correct one.

CREATE TABLE follows (
    -- The lifter being followed, and the one doing the following. Two references
    -- to the same table, and they are always different people: the CHECK below
    -- says so, because following yourself is a row that could only ever mean what
    -- the default already means.
    --
    -- Both cascade, following 0024's reasoning exactly. Removing an account takes
    -- the follows pointing at it AND the ones it made — neither is training
    -- history, the one thing in this database that cannot be recomputed, so
    -- neither needs the careful refusal the program down-migrations make.
    followee_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    follower_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Cheap, and it makes the handler's 403 a belt to this brace rather than the
    -- only thing standing between the table and a meaningless row.
    CONSTRAINT follows_not_self_ck CHECK (follower_id <> followee_id),

    -- THE PRIMARY KEY IS THE IDEMPOTENCY, which is session_reactions' reasoning in
    -- 0024 and applies here word for word. A lifter pressing Follow twice means
    -- one follow, and that is settled by the database rather than by a handler
    -- remembering to look first — which under concurrent taps it could not do
    -- correctly anyway. The API inserts with ON CONFLICT DO NOTHING and a second
    -- press is a no-op rather than a 409, because pressing a button twice is not a
    -- mistake the lifter should be told about.
    --
    -- COLUMN ORDER IS THE HOT READ, again as 0024 argues. The crown fan-out asks
    -- "who follows this actor", which needs followee_id leading, and it runs inside
    -- the hourly reconciler's transaction where it is the only thing standing
    -- between a reign opening and the install hearing about it. The is_following
    -- check on the roster and the profile binds both columns, so it is an exact
    -- lookup either way round.
    PRIMARY KEY (followee_id, follower_id)
);

-- No second index, unlike 0024 — and that is a statement rather than an omission.
--
-- session_reactions needed one because "everything this lifter has reacted to" is a
-- real read with the wrong leading column. Nothing here asks the mirror question:
-- there is no "who do I follow" list, and the roster answers per row with both
-- columns bound. The day a following list exists it wants
-- `CREATE INDEX follows_follower_idx ON follows (follower_id)`, and until then an
-- index nothing reads is a write cost on every press.

COMMIT;
