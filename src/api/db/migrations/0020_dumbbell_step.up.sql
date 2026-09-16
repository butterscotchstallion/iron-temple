BEGIN;

-- What this lifter's dumbbell rack steps by.
--
-- 0013 recorded the bar and the plates because "the numbers this app puts on
-- the bar have to be numbers this gym can build". It recorded nothing about
-- dumbbells, and the engine has been guessing ever since: DumbbellIncrementLb
-- is a hard 10 lb in progression.go, reasoned from a rack that steps 5 lb a
-- bell. That is the common rack, not the only one, and the guess is expensive
-- in exactly one place — assistance work, which advances by the smallest change
-- its equipment admits. A curl topping out its rep range goes 30 lb to 40 on
-- the pair, a third heavier in one session, because the app believes nothing
-- finer exists.
--
-- Stored PER BELL rather than per pair. A rack is labelled in bells, a lifter
-- reads it in bells, and the doubling is arithmetic the app can do; storing the
-- pair would mean every lifter entering a number their equipment does not have
-- written on it. Every weight the app displays is still the whole load — see
-- 0018 — so the pair steps twice this.
--
-- DEFAULT 5 is the rack 0013's generation assumed, which makes this column a
-- no-op for every account that already exists and every account that never
-- opens the setup screen: 5 a bell is 10 on the pair, exactly what the engine
-- hardcoded. The default is doing the same job it does for bar_weight_lb —
-- what a rack steps by when you do not know anything else about it.
--
-- CHECK (> 0) and nothing tighter. The bound that matters — no half-pound
-- jumps on a pair of dumbbells — is a claim about plausible equipment rather
-- than about coherent data, so it lives in the handler beside maxBarWeightLb,
-- where 0013's bar bound already sits and where a rejected value can say why.
ALTER TABLE user_gym
    ADD COLUMN dumbbell_step_lb NUMERIC(5,2) NOT NULL DEFAULT 5
        CHECK (dumbbell_step_lb > 0);

COMMIT;
