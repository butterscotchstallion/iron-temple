BEGIN;

-- What the rest of this lifter's equipment steps by, and whether they have ever
-- actually told us any of it.
--
-- WHY A STEP PER KIND
--
-- 0013 recorded the bar and the plates, 0020 recorded the dumbbell rack's step,
-- and everything else has been answered by the BAR this whole time. Not as a
-- documented default — as the else-branch of a two-way if. progression.stepFor
-- asks "is this a dumbbell?", and every other kind the catalogue admits, machine
-- and cable and band and bodyweight alike, takes GymSteps.BarLb: twice the
-- lightest plate the lifter happens to own.
--
-- That is a false sentence about a gym. It says buying a pair of 1.25 lb plates
-- makes the leg press finer, and it does not: a stack is a stack, and the pin
-- drops into holes somebody else drilled. The lifter who owns fine plates is
-- rewarded with a cable prescription of 43.75 lb, which no pin makes.
--
-- Three columns and not one, because they are three different facts that only
-- look alike at the default. A selectorized stack usually steps 10 or 15 with a
-- 2.5 add-on; a cable stack is often 5; a band set jumps by whatever the
-- manufacturer graded it. DEFAULT 5 for all three is what the engine already
-- behaves like for an account with a standard plate set, so this migration
-- changes nobody's prescription on the day it lands — which is the bar every
-- backfill here has to clear.
--
-- Bodyweight gets no column. There is nothing to configure: the load on a
-- weighted dip is a plate on a belt or a bell between the feet, and both of
-- those inventories are recorded already. It keeps taking the bar's, as it
-- always has.
--
-- WHY equipment_confirmed_at
--
-- Because the app has been guessing gyms since 0013 and has never once asked
-- anybody to check. That migration seeded every existing account with "the
-- standard home-gym set, two pairs of each" and called it "a guess, but a
-- checkable one: it is visible and editable on the profile screen the moment
-- this ships". Checkable it was. Checked it was not — SeedDefaultPlates has gone
-- on handing the same six denominations to every account created since, and a
-- lifter on this install found the plate calculator offering them a 35 lb plate
-- they have never owned. The guess was indistinguishable from a fact because
-- nothing recorded which one it was.
--
-- This column records it. NULL means the rows describing this gym were written
-- by us and nobody has looked at them; a timestamp means a lifter opened the
-- equipment screen and said yes, this is my gym. It stays NULL for every account
-- that exists today, including the one that reported the plate, because that is
-- the truth about all of them.
--
-- IT MUST NOT GATE THE ENGINE. This is the obvious wrong use of the column and
-- the reason it is spelled out here. An unconfirmed rack is still the best
-- information available — it is a plausible gym, and for most lifters it is
-- their actual one — so prescribing differently until somebody clicks a button
-- would mean two lifters with identical equipment get different weights. The
-- flag drives copy: a banner, a nudge, a first-run step. Never arithmetic.
--
-- NULLABLE, with no DEFAULT, deliberately. sessions.finished_at (0004) and
-- report_runs.sent_at (0008) are the precedent: a timestamp that is absent until
-- the thing happens, rather than a boolean that has to be kept in step with one.
--
-- Note a user_gym row's ABSENCE is the normal state, not an error — register()
-- has never created one, and GetBarWeight answers over a LEFT JOIN so a missing
-- row still gives a truthful bar. That works in our favour here: an account with
-- no row reads as unconfirmed, which is exactly what a brand-new account is.
--
-- Bounds are CHECK (> 0) and nothing tighter, following 0020: the plausibility
-- limits ("no half-pound jumps on a cable stack") are a claim about equipment
-- rather than about coherent data, so they live in the Go handler beside
-- maxBarWeightLb where a rejected value can say why it was rejected.

ALTER TABLE user_gym
    ADD COLUMN machine_step_lb NUMERIC(5,2) NOT NULL DEFAULT 5
        CHECK (machine_step_lb > 0),
    ADD COLUMN cable_step_lb NUMERIC(5,2) NOT NULL DEFAULT 5
        CHECK (cable_step_lb > 0),
    ADD COLUMN band_step_lb NUMERIC(5,2) NOT NULL DEFAULT 5
        CHECK (band_step_lb > 0),
    ADD COLUMN equipment_confirmed_at TIMESTAMPTZ;

COMMIT;
