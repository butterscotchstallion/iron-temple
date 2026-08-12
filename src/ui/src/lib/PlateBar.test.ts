import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import PlateBar from "./PlateBar.svelte";

// BAR_LB is 80, so anything at/below 80 loads no plates.
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
