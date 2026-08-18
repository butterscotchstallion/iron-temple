import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import BodyweightCard from "./BodyweightCard.svelte";

const box = () => screen.getByRole("spinbutton") as HTMLInputElement;
const caption = () => screen.getByTestId("bodyweight-caption");

// Commit the box the way a lifter does: type, then blur/Enter. fireEvent.input
// alone only moves the DOM value; the component saves on change.
async function enter(value: string) {
  await fireEvent.input(box(), { target: { value } });
  await fireEvent.change(box(), { target: { value } });
}

describe("BodyweightCard", () => {
  it("opens empty when nothing has ever been weighed", () => {
    render(BodyweightCard, { onSave: vi.fn() });
    expect(box()).toHaveValue(null);
    expect(caption()).toHaveTextContent("First weigh-in");
  });

  it("pre-fills the last weigh-in and says where it came from", () => {
    render(BodyweightCard, {
      lastWeighIn: { weightLb: 184.5, performedOn: "2026-08-14" },
      onSave: vi.fn(),
    });
    expect(box()).toHaveValue(184.5);
    expect(caption()).toHaveTextContent("Carried from August 14 2026");
  });

  it("prefers this session's own weigh-in over the carried one", () => {
    render(BodyweightCard, {
      bodyweightLb: 182,
      lastWeighIn: { weightLb: 184.5, performedOn: "2026-08-14" },
      onSave: vi.fn(),
    });
    expect(box()).toHaveValue(182);
    expect(caption()).toHaveTextContent("Logged for this session");
  });

  it("saves an edited weight", async () => {
    const onSave = vi.fn();
    render(BodyweightCard, {
      lastWeighIn: { weightLb: 184.5, performedOn: "2026-08-14" },
      onSave,
    });

    await enter("183.5");
    expect(onSave).toHaveBeenCalledWith(183.5);
  });

  // The point of "carried, not copied": showing last week's number must not
  // write it onto this session. Only an edit does, and in a browser only an edit
  // fires change — a blur that leaves the value untouched fires nothing.
  it("does not save a carried value just by showing it", () => {
    const onSave = vi.fn();
    render(BodyweightCard, {
      lastWeighIn: { weightLb: 184.5, performedOn: "2026-08-14" },
      onSave,
    });

    expect(box()).toHaveValue(184.5);
    expect(onSave).not.toHaveBeenCalled();
  });

  // Reachable after a failed save: the box still shows what was typed, and
  // typing the stored value back is a change event that asks for nothing.
  it("does not re-save a weight the session already holds", async () => {
    const onSave = vi.fn();
    render(BodyweightCard, { bodyweightLb: 182, onSave });

    await fireEvent.change(box(), { target: { value: "182" } });
    expect(onSave).not.toHaveBeenCalled();
  });

  it("clears a recorded weigh-in when the box is emptied", async () => {
    const onSave = vi.fn();
    render(BodyweightCard, { bodyweightLb: 182, onSave });

    await enter("");
    expect(onSave).toHaveBeenCalledWith(null);
  });

  it("emptying a merely pre-filled box saves nothing", async () => {
    const onSave = vi.fn();
    render(BodyweightCard, {
      lastWeighIn: { weightLb: 184.5, performedOn: "2026-08-14" },
      onSave,
    });

    await enter("");
    expect(onSave).not.toHaveBeenCalled();
  });

  it("refuses an out-of-range weight and says so", async () => {
    const onSave = vi.fn();
    render(BodyweightCard, { onSave });

    await enter("7");
    expect(onSave).not.toHaveBeenCalled();
    expect(caption()).toHaveTextContent("between 50 and 1000 lb");

    await enter("1200");
    expect(onSave).not.toHaveBeenCalled();

    // A good entry clears the complaint. The caption falls back to the
    // first-weigh-in line rather than "Logged", because onSave is a stub here:
    // the card reads its state from the props, and nothing updated them.
    await enter("184");
    expect(onSave).toHaveBeenCalledWith(184);
    expect(caption()).not.toHaveTextContent("between 50 and 1000 lb");
  });

  it("locks an over session and shows its recorded weight", () => {
    render(BodyweightCard, { bodyweightLb: 182, readonly: true, onSave: vi.fn() });
    expect(box()).toBeDisabled();
    expect(box()).toHaveValue(182);
  });

  // A locked box showing a carried number would read as a measurement that was
  // never taken.
  it("shows no carried value on an over session that recorded none", () => {
    render(BodyweightCard, {
      lastWeighIn: { weightLb: 184.5, performedOn: "2026-08-14" },
      readonly: true,
      onSave: vi.fn(),
    });
    expect(box()).toHaveValue(null);
    expect(caption()).toHaveTextContent("No weigh-in");
  });
});
