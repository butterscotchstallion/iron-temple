import { fireEvent, render, screen, waitFor, within } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Profile from "./Profile.svelte";
import { auth } from "../lib/auth.svelte";
import { testUser } from "../lib/testFixtures";

// The equipment area. This screen is where a lifter tells the app what their gym
// actually is, and it exists because the app had been guessing since 0013 and
// had never once asked — which is how somebody was offered a 35 lb plate they
// have never owned.
//
// Two things are worth pinning here and are not covered anywhere else: that the
// unconfirmed banner appears exactly when the gym is still the app's guess, and
// that saving sends the three stacks — the fields with no other home, since
// there is no inventory of pins to derive them from.

const updateMe = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  updateMe,
}));

const BANNER = /never checked this with you/i;

// This screen is three independent forms, each with its own Save — grouping
// them behind one button would mean a rejected password discards a display-name
// edit. So "the Save button" is ambiguous, and the equipment one is found by
// the form it sits in rather than by being the second of two.
function equipmentSave(): HTMLElement {
  const form = screen.getByLabelText("Machines").closest("form");
  if (!form) throw new Error("the machine step input is not inside a form");
  return within(form).getByRole("button", { name: "Save" });
}

function signIn(over: Parameters<typeof testUser>[0] = {}) {
  auth.me = testUser({
    barWeightLb: 45,
    dumbbellStepLb: 5,
    machineStepLb: 5,
    cableStepLb: 5,
    bandStepLb: 5,
    plates: [{ plateLb: 45, pairs: 2 }],
    equipmentConfirmedAt: null,
    ...over,
  });
  auth.loaded = true;
}

beforeEach(() => {
  vi.clearAllMocks();
  signIn();
  updateMe.mockImplementation(async (body) => ({
    status: 200,
    data: { ...auth.me, ...body, equipmentConfirmedAt: "2026-09-23T00:00:00Z" },
  }));
});

describe("Profile equipment", () => {
  it("asks an unconfirmed lifter to check the gym we invented for them", () => {
    render(Profile);
    expect(screen.getByText(BANNER)).toBeInTheDocument();
  });

  // The other side of the same branch. A lifter who has already said yes is not
  // asked again — a prompt that never goes away is one people learn to skip.
  it("says nothing to a lifter who has already confirmed", () => {
    signIn({ equipmentConfirmedAt: "2026-09-01T00:00:00Z" });
    render(Profile);
    expect(screen.queryByText(BANNER)).not.toBeInTheDocument();
  });

  // Three fields and not one: a selectorized stack, a cable stack and a graded
  // band set are three different facts that only look alike at the default.
  it("offers a step for each kind of stack", () => {
    render(Profile);
    expect(screen.getByLabelText("Machines")).toHaveValue(5);
    expect(screen.getByLabelText("Cables")).toHaveValue(5);
    expect(screen.getByLabelText("Bands")).toHaveValue(5);
  });

  it("sends the stacks when the gym is saved", async () => {
    render(Profile);

    await fireEvent.input(screen.getByLabelText("Machines"), {
      target: { value: "15" },
    });
    await fireEvent.click(equipmentSave());

    await waitFor(() => expect(updateMe).toHaveBeenCalled());
    expect(updateMe).toHaveBeenCalledWith(
      expect.objectContaining({
        machineStepLb: 15,
        cableStepLb: 5,
        bandStepLb: 5,
      }),
    );
  });

  // Saving IS the confirmation — there is no separate "yes, this is right"
  // button, because a button like that is one more thing to not press. The
  // banner going away is how the lifter sees that it took.
  it("stops asking once the gym has been saved", async () => {
    render(Profile);
    expect(screen.getByText(BANNER)).toBeInTheDocument();

    await fireEvent.click(equipmentSave());

    await waitFor(() => {
      expect(screen.queryByText(BANNER)).not.toBeInTheDocument();
    });
  });
});
