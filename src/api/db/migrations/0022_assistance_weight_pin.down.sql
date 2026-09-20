BEGIN;

-- Dropping the column loses every unspent pin, and the loss is visible: a
-- lifter who set a curl back to 30 yesterday and has not trained it since goes
-- back to being prescribed the 50 they were trying to get away from. Nothing
-- else can be done, because the pin has no other home — weight_lb still holds
-- the number they asked for, but without the watermark there is no way to know
-- it outranks the carry-forward rather than predating it.
--
-- A spent pin loses nothing, which is most of them. Once the lift has been
-- performed again the column no longer matches the latest session and stopped
-- affecting the prescription; dropping it and leaving the carry-forward to
-- answer on its own produces the same weight either way.
ALTER TABLE program_day_assistance DROP COLUMN weight_set_after_session_id;

COMMIT;
