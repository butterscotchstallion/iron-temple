import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import PlateBar from "./PlateBar.svelte";
import { auth } from "./auth.svelte";
import type { User } from "./api";

// The bar and the rack come off the profile now, so these have to sign in as
// somebody with a gym. An 80 lb bar and the standard rack, which is what this
// file's arithmetic has always assumed — it just used to be a constant.
const lifter = {
  id: 1,
  username: "ada",
  displayName: "Ada",
  avatarColor: "",
  isAdmin: true,
  hasAvatar: false,
  barWeightLb: 80,
  plates: [
    { plateLb: 45, pairs: 2 },
    { plateLb: 35, pairs: 2 },
    { plateLb: 25, pairs: 2 },
    { plateLb: 10, pairs: 2 },
    { plateLb: 5, pairs: 2 },
    { plateLb: 2.5, pairs: 2 },
  ],
} satisfies User;

beforeEach(() => {
  auth.me = { ...lifter };
  auth.loaded = true;
});

afterEach(() => {
  auth.me = null;
  auth.loaded = false;
});

describe("PlateBar", () => {
  it("shows 'just the bar' at or below bar weight", () => {
    render(PlateBar, { weightLb: 80 });
    expect(screen.getByText("just the bar")).toBeInTheDocument();
    expect(screen.queryByTitle(/lb$/)).toBeNull();
  });

  it("labels the bar for assistive tech", () => {
    const { container } = render(PlateBar, { weightLb: 230 });
    expect(container.querySelector('[aria-label="Barbell loaded to 230 lb"]')).not.toBeNull();
  });

  it("renders a single plate per side for a one-plate weight", () => {
    // (170 - 80) / 2 = 45 per side.
    render(PlateBar, { weightLb: 170 });
    expect(screen.queryByText("just the bar")).toBeNull();
    // One disc on each side (left mirror + right).
    expect(screen.getAllByTitle("45 lb")).toHaveLength(2);
  });

  it("greedily loads the largest plates first, mirrored on both sides", () => {
    // (230 - 80) / 2 = 75 = 45 + 25 + 5 per side.
    render(PlateBar, { weightLb: 230 });
    expect(screen.getAllByTitle("45 lb")).toHaveLength(2);
    expect(screen.getAllByTitle("25 lb")).toHaveLength(2);
    expect(screen.getAllByTitle("5 lb")).toHaveLength(2);
    // No 35s or 10s were used.
    expect(screen.queryByTitle("35 lb")).toBeNull();
    expect(screen.queryByTitle("10 lb")).toBeNull();
  });
});
