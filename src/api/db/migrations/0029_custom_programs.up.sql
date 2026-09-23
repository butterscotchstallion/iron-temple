BEGIN;

-- Programs a lifter builds for themselves.
--
-- 0009 opened with the constraint that has shaped everything since: "PROGRAMS
-- ARE NEVER EDITED. programs, program_days and program_day_exercises stay
-- exactly as seeded — shared between every account, and canonical." That rule
-- bought two things. The progression engine reads a prescription nobody has
-- touched, so a squat advances identically for every account; and assistance
-- could be an additive per-user overlay rather than a mutation, so what one
-- lifter bolts onto Workout A is invisible to the next.
--
-- This migration does NOT repeal that rule. It narrows it, and the narrowing is
-- the same one 0009 itself performed on the exercise library one section
-- earlier: an owner column where NULL means seeded.
--
--     created_by_user_id IS NULL      the install's. Shared, canonical, and
--                                     editable by nobody — not even the admin.
--     created_by_user_id = <lifter>   one lifter's own. Theirs to edit.
--
-- The eight seeded programs are exactly as immutable after this migration as
-- before it, and by construction rather than by a check somebody has to
-- remember to write: every write path added on top of this scopes on
-- created_by_user_id = :caller, and NULL = anything is never true in SQL. That
-- is the same trick DeleteExercise already uses to make the seeded catalogue
-- undeletable, and it is why there is no is_seeded boolean here — a nullable
-- owner already says it, and two columns that must agree eventually disagree.
--
-- What the overlay keeps meaning: program_day_assistance is untouched and still
-- works, on custom programs as on seeded ones. It is now the *second* way to
-- change what you lift rather than the only one, and the two do not collide —
-- prescribe() already skips an assistance row naming a lift the day prescribes,
-- with a comment anticipating exactly this ("a seed migration adding a lift to a
-- day would create the same collision from the other side"). A lifter following
-- somebody's shared program who has curls as assistance, and whose owner then
-- promotes curls into the prescription, sees the lift once.

-- ---------------------------------------------------------------------------
-- 1. Ownership, visibility, and archival.
-- ---------------------------------------------------------------------------

ALTER TABLE programs
    -- No ON DELETE clause, and deliberately. 0009 left the same column bare on
    -- exercises because CASCADE would take a lifter's custom movements with
    -- them and orphan logged history; here the reason is sharper still.
    --
    -- CASCADE would run programs -> program_days -> program_day_exercises and
    -- then reach sessions.program_day_id, which has no ON DELETE and therefore
    -- RESTRICTs. The delete would fail PART WAY THROUGH A CASCADE, which is the
    -- worst of both answers. SET NULL is worse again in a quieter way: it would
    -- promote a departed lifter's private program to a canonical seeded one,
    -- visible to and uneditable by the whole install.
    --
    -- There is no account-deletion path for real lifters today. The generated-
    -- activity teardown does delete accounts, so deleteActivity has to decide
    -- what happens to a program one of them owns; in practice they never create
    -- one, so the FK error is the backstop and the guard is in the handler.
    ADD COLUMN created_by_user_id INTEGER REFERENCES users(id),

    -- Whether the rest of the install can find this program.
    --
    -- DEFAULT false with an explicit backfill below, rather than DEFAULT true
    -- and no backfill. The default that gets used from here on is the one a new
    -- CUSTOM program takes, and private is the only safe answer there. The
    -- seeded rows are handled by the sentence below, which is also the one
    -- readers will look for.
    --
    -- Note that the visibility predicate ignores this column entirely for a
    -- NULL owner (created_by_user_id IS NULL OR is_shared OR owner = caller),
    -- so a future seed migration that forgets to set it still ships a visible
    -- program. The backfill is for tidiness and for anyone reading the table by
    -- hand, not for correctness.
    ADD COLUMN is_shared BOOLEAN NOT NULL DEFAULT false,

    -- When the owner retired it. NULL for a live program.
    --
    -- Archival and not deletion because deletion is not available: a program
    -- whose days have sessions against them cannot be removed while
    -- sessions.program_day_id RESTRICTs, and making that FK cascade would trade
    -- a tidy catalogue for silently destroyed history — the tonnage, the PRs and
    -- every Racked figure computed from those sets. So this column is the honest
    -- version of what the schema already enforces.
    --
    -- Archiving controls DISCOVERY, not access. An archived program leaves the
    -- picker but still resolves, still previews and can still be trained, so a
    -- lifter part-way through one is never locked out of it. users
    -- .current_program_id is ON DELETE SET NULL, which an archive does not fire
    -- — that is fine precisely because the program keeps resolving.
    ADD COLUMN archived_at TIMESTAMPTZ;

-- Every row that exists when this runs is seeded, and the seeded programs are
-- the install's: everyone sees them.
UPDATE programs SET is_shared = true;

-- ---------------------------------------------------------------------------
-- 2. Name uniqueness becomes per-owner.
-- ---------------------------------------------------------------------------
--
-- Straight from 0009's treatment of exercises, for the same reason: two lifters
-- may each have a program called "Push Pull Legs"; neither may have two. And
-- case-insensitively, matching the users_username_lower_idx precedent in 0005 —
-- a picker listing both "Push Pull Legs" and "push pull legs" is a picker with a
-- bug in it.
--
-- The eight seeded names are distinct under lower(), so the swap is safe.

ALTER TABLE programs DROP CONSTRAINT programs_name_key;

CREATE UNIQUE INDEX programs_name_shared_idx
    ON programs (lower(name))
    WHERE created_by_user_id IS NULL;

CREATE UNIQUE INDEX programs_name_owned_idx
    ON programs (lower(name), created_by_user_id)
    WHERE created_by_user_id IS NOT NULL;

-- Every program read filters on the owner ("seeded, mine, or shared").
CREATE INDEX programs_owner_idx ON programs (created_by_user_id);

-- Note what is NOT in those predicates: is_shared.
--
-- Keying uniqueness on a mutable column would mean that ticking the share box
-- can fail with a constraint violation about a DIFFERENT LIFTER'S program name
-- — an error the owner can neither understand nor act on, since they cannot see
-- the row it is about and cannot rename it. Two lifters may both share a program
-- called "My Program". The picker tells them apart by owner, which is what the
-- owner's display name on the wire is for.
--
-- Carried forward from 0009, and it applies to programs now too: a future seed
-- migration CANNOT use ON CONFLICT (name) the way 0002, 0007, 0012, 0018 and
-- 0023 do, because that constraint no longer exists and neither partial index is
-- inferrable from a bare column list. Use WHERE NOT EXISTS. (Those five are safe
-- — they all run before this migration.)

-- ---------------------------------------------------------------------------
-- 3. Days archive too, because a day that has been trained cannot be deleted.
-- ---------------------------------------------------------------------------
--
-- Same RESTRICT as above, one level down: sessions.program_day_id points at a
-- program_days row, so removing a day somebody has trained is not on offer. The
-- editor still has to let an owner take a day out of a program, so it archives.

ALTER TABLE program_days ADD COLUMN archived_at TIMESTAMPTZ;

-- Which forces both uniques to become partial. An archived "Workout B" would
-- otherwise occupy its name and its position for ever: an owner who removed a
-- day could not add another with the same name, and the position it left behind
-- could never be reused. Restricting the constraints to live rows says what was
-- always meant — these are rules about the program as it stands, not about every
-- row that has ever been part of it.
--
-- lower(name) on the way past, matching the case-insensitivity 0005 and 0009
-- settled on everywhere else.

ALTER TABLE program_days DROP CONSTRAINT program_days_program_id_name_key;
ALTER TABLE program_days DROP CONSTRAINT program_days_program_id_position_key;

CREATE UNIQUE INDEX program_days_name_live_idx
    ON program_days (program_id, lower(name))
    WHERE archived_at IS NULL;

CREATE UNIQUE INDEX program_days_position_live_idx
    ON program_days (program_id, position)
    WHERE archived_at IS NULL;

-- ---------------------------------------------------------------------------
-- 4. program_day_exercises gets no archived_at, and the asymmetry is the point.
-- ---------------------------------------------------------------------------
--
-- It would be natural to assume the prescription rows need the same treatment
-- the days just got. They do not, and the difference is worth stating so the
-- next reader does not "fix" it.
--
-- Nothing references a program_day_exercises row except
-- program_day_exercise_sets, which cascades. session_sets references
-- exercise_id — the MOVEMENT — and never the prescription. So deleting a lift
-- from a day is a plain DELETE that cannot fail on a foreign key and cannot lose
-- a single logged set: the work stays in session_sets, keeps counting toward
-- volume and records, and only the plan changes. That is exactly the property
-- program_day_assistance was built on ("deleting a row deletes a plan, never a
-- performance"), and it falls out of 0001's design for free.
--
-- Position gaps left by a delete are fine. Every read is ORDER BY position and
-- nothing assumes the numbers are contiguous.

COMMIT;
