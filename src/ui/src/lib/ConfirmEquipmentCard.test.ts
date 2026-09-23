import { fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, it } from "vitest";
import ConfirmEquipmentCard from "./ConfirmEquipmentCard.svelte";
import { auth } from "./auth.svelte";
import { testUser } from "./testFixtures";

// The nudge that gets a lifter to the equipment screen at all.
//
// It exists because the banner on the profile can only be read by somebody
// already on the profile, and the lifter this is for has no reason to go there:
// a seeded rack looks exactly like a described one. That is the whole bug.

const NUDGE = /never told us whether that's right/i;

afterEach(() => {
  auth.me = null;
  auth.loaded = false;
});

function signIn(equipmentConfirmedAt: string | null) {
  auth.me = testUser({ equipmentConfirmedAt });
  auth.loaded = true;
}

describe("ConfirmEquipmentCard", () => {
  it("asks a lifter whose gym is still our guess", () => {
    signIn(null);
    render(ConfirmEquipmentCard);
    expect(screen.getByText(NUDGE)).toBeInTheDocument();
  });

  it("draws nothing once the gym has been confirmed", () => {
    signIn("2026-09-01T00:00:00Z");
    render(ConfirmEquipmentCard);
    expect(screen.queryByText(NUDGE)).not.toBeInTheDocument();
  });

  // Home renders this above the workout, so it has to be possible to clear it
  // and read the screen you came for.
  //
  // "Later", not "Not now": the update prompt owns that name and can be on Home
  // at the same time, and two same-named buttons on one screen is ambiguous to
  // anyone reading by name rather than position.
  it("can be dismissed", async () => {
    signIn(null);
    render(ConfirmEquipmentCard);

    await fireEvent.click(screen.getByRole("button", { name: /later/i }));

    expect(screen.queryByText(NUDGE)).not.toBeInTheDocument();
  });

  // Nothing to nudge on a screen with no lifter on it yet — and in particular
  // this must not flash up during the moment before /me lands, which is exactly
  // the kind of guess-from-nothing the rest of this change removes.
  it("draws nothing before a profile has loaded", () => {
    auth.me = null;
    render(ConfirmEquipmentCard);
    expect(screen.queryByText(NUDGE)).not.toBeInTheDocument();
  });
});
