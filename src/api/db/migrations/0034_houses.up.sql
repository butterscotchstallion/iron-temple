BEGIN;

-- Houses: a named group of lifters, and the sigil its members wear.
--
-- An install is a flat list of accounts. The owner adds them by hand, everybody
-- can already see everybody else's training (see internal/api/lifters.go), and
-- nothing anywhere says which of them train together. A household with two
-- lifters does not need to be told; a gym crew of nine does.
--
-- WHY THIS CHANGES NO EXISTING READ
--
-- lifters.go states at length that there is no per-lifter visibility filter and
-- that its absence is a decision rather than an omission: admission to the
-- install is the consent. A House could have been the seam where that premise
-- was reversed — members see each other, strangers do not — and it deliberately
-- is not. Nothing that is visible today becomes hidden here. Not one query in
-- this repo grows a house_id predicate.
--
-- What a House adds is grouping, not gating: a sigil beside a name, a page that
-- collects the members, and a way in. That keeps this migration additive, and it
-- keeps the answer to "who may read this" in one place instead of two.
--
-- WHY THE SIGIL IS NOT A COLUMN ON users
--
-- Because it is not a property of a lifter. It is a property of the House they
-- are in, and copying it onto the member would mean a rename touching every row
-- rather than one. The practical half matters more: db/queries/users.sql:143-147
-- requires that a column added to ListLifters is added to GetLifter too, and six
-- more queries hydrate a lifter for the wire. A join on all eight to draw an
-- ornament is the cost the crown in 0032 refused to pay, and this refuses it the
-- same way — one site-wide read, consulted per name.

CREATE TABLE houses (
    id SERIAL PRIMARY KEY,

    -- Both unique case-insensitively, via the indexes below. Two Houses called
    -- "Iron Temple" and "iron temple" on one install is a mistake being made,
    -- not a distinction being drawn.
    name  TEXT NOT NULL,

    -- The tag worn beside a member's name. Short because it is rendered inline
    -- in a feed row, a comment byline and a notification panel, next to a name
    -- that is already truncating.
    sigil TEXT NOT NULL,

    -- The one-line pitch. Shown in the hover card and on the detail page, so it
    -- has to survive being read in a 240px-wide popover on a phone.
    tagline     TEXT NOT NULL DEFAULT '',

    -- The long version, shown only on the detail page. Nothing else reads it,
    -- which is why the site-wide GET /houses leaves it out of the payload every
    -- client fetches on load.
    description TEXT NOT NULL DEFAULT '',

    -- A name from the closed set the OpenAPI HouseIcon enum declares, and a hex
    -- colour in the form users.avatar_color already uses.
    --
    -- Picked rather than uploaded, which is the whole reason there is no
    -- house_icons table beside user_avatars: an icon that is two short strings
    -- costs no storage, needs no serving endpoint with its own ETag, and can be
    -- drawn inside a hover card that is itself drawn from a list already in
    -- memory. An uploaded image would be a request per House per card.
    --
    -- Not CHECKed against the set, for 0026's reason: the list would then exist
    -- in the spec, in Go, and here, and the third copy is the one that goes
    -- stale silently. Empty is allowed and means "no icon chosen".
    icon       TEXT NOT NULL DEFAULT '',
    icon_color TEXT NOT NULL DEFAULT '',

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Two to five alphanumerics. The ceiling is the interesting half: this is
    -- rendered inline beside a name, so a long sigil does not look bad, it
    -- pushes the name it belongs to out of the row.
    CONSTRAINT houses_sigil_ck CHECK (sigil ~ '^[A-Za-z0-9]{2,5}$'),

    -- Lengths rather than shapes, because prose is prose. Both are ceilings the
    -- UI also enforces; this is the one that cannot be bypassed by calling the
    -- API directly.
    CONSTRAINT houses_tagline_ck     CHECK (char_length(tagline) <= 80),
    CONSTRAINT houses_description_ck CHECK (char_length(description) <= 2000)
);

-- lower() rather than a citext column, matching how users.username is kept
-- unique. One fewer extension to install on a box somebody else runs.
CREATE UNIQUE INDEX houses_name_idx  ON houses (lower(name));
CREATE UNIQUE INDEX houses_sigil_idx ON houses (lower(sigil));

-- Membership, and the whole of "one House at a time".
--
-- user_id is the PRIMARY KEY, not a column with an index on it. That is the
-- rule expressed as a shape: a lifter in two Houses is not something the
-- handlers have to remember to refuse, it is a row the database cannot hold. It
-- matters because the sigil is drawn beside a name from a map keyed by lifter
-- id, and a lifter with two Houses would make that map's value ambiguous
-- everywhere it is read.
CREATE TABLE house_members (
    user_id  INTEGER PRIMARY KEY REFERENCES users(id)  ON DELETE CASCADE,
    house_id INTEGER NOT NULL    REFERENCES houses(id) ON DELETE CASCADE,

    -- Ownership lives here rather than as houses.owner_id, and the difference is
    -- not cosmetic. On the membership row, an owner who is not a member becomes
    -- unrepresentable, and an account being deleted takes its ownership with it
    -- through the cascade already on this table rather than leaving houses
    -- pointing at a user that is gone.
    --
    -- What it cannot express is who owns a House whose owner deleted their
    -- account: the cascade removes the row and no transfer runs. That case is
    -- resolved by reading, not by writing — see the effective-owner rule in
    -- internal/api/houses.go — so there is no state here that needs repairing on
    -- a schedule.
    is_owner BOOLEAN NOT NULL DEFAULT false,

    joined_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The members of one House: the detail page's read, and the fan-out for a
-- notification addressed to a House.
CREATE INDEX house_members_house_idx ON house_members (house_id);

-- At most one owner per House. Partial on the boolean, the shape 0029, 0030 and
-- 0032 all use, so the index stays the size of the House count rather than the
-- membership count.
CREATE UNIQUE INDEX house_members_owner_idx
    ON house_members (house_id)
    WHERE is_owner;

-- Asking to join, and what came of it.
--
-- A ledger, not a queue: rows are decided, never deleted. 0032 kept closed
-- reigns for the same reason — "asked and was turned down" is not derivable from
-- anything else in the database, and a lifter who wants to know whether they
-- ever asked has nowhere else to look. It also makes a second request after a
-- decline an ordinary insert rather than an update that erases the first.
CREATE TABLE house_join_requests (
    id SERIAL PRIMARY KEY,

    house_id INTEGER NOT NULL REFERENCES houses(id) ON DELETE CASCADE,
    user_id  INTEGER NOT NULL REFERENCES users(id)  ON DELETE CASCADE,

    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- NULL means pending. A timestamp rather than a boolean for 0026's reason:
    -- it costs the same and answers "when", which a boolean cannot be asked
    -- later.
    decided_at TIMESTAMPTZ,

    -- Who decided. SET NULL rather than CASCADE: the decision outlives the
    -- account that made it, and losing the row would rewrite the requester's
    -- history because an owner closed their account.
    decided_by INTEGER REFERENCES users(id) ON DELETE SET NULL,

    -- NULL while pending, then one of 'approved', 'declined', 'withdrawn',
    -- 'superseded'. 'superseded' is the one worth naming: a lifter may ask
    -- several Houses, the first approval wins, and the rest are closed by that
    -- transaction rather than left pending against a lifter who can no longer
    -- accept them.
    --
    -- No CHECK on the values, for the reason the icon column gives above.
    outcome TEXT,

    -- Decided and undecided are the same fact stated twice, so they cannot be
    -- allowed to disagree. This is what stops a half-written decision — an
    -- outcome with no timestamp, or the reverse — from reading as pending
    -- forever.
    CONSTRAINT house_join_requests_decision_ck
        CHECK ((decided_at IS NULL) = (outcome IS NULL))
);

-- One OPEN request per lifter per House. Partial, so a lifter who was declined
-- in March can ask again in April, which a plain unique index would forbid.
CREATE UNIQUE INDEX house_join_requests_open_idx
    ON house_join_requests (house_id, user_id)
    WHERE decided_at IS NULL;

-- What an owner is shown: the pending requests for their House.
CREATE INDEX house_join_requests_pending_idx
    ON house_join_requests (house_id)
    WHERE decided_at IS NULL;

-- ---- notifications ----
--
-- A request nobody is told about is a request nobody answers, so this feature
-- cannot ship without the fan-out. The subject column it needs is which House,
-- and it is the fifth nullable subject on this table — session_id, comment_id,
-- emoji and achievement_slug are all nullable for the same reason 0026 gave:
-- not every kind has every subject.
--
-- Note which way each of the three kinds points. notifications.actor_id is NOT
-- NULL and every insert filters the actor out of the recipients, so a request
-- tells the owner and not the requester, and a decision tells the requester and
-- not the owner. Nobody is ever told what they just did.
ALTER TABLE notifications
    ADD COLUMN house_id INTEGER REFERENCES houses(id) ON DELETE CASCADE;

-- No index for it, for 0032's reason: the panel's reads filter on user_id and
-- group on (kind, session_id), and this column is only ever read back out of a
-- row one of those already found.

-- No seed. Unlike 0032's catalogue there is nothing here that the install owns —
-- every House is founded by a lifter, and an install where nobody has founded
-- one shows an empty list, which is the correct answer rather than a missing
-- one.

COMMIT;
