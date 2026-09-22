import { render, screen, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Astroturfing from "./Astroturfing.svelte";

// The screen the generated-activity controls moved onto, out of /admin.
//
// Thin by design — it is a heading and the panel, and the panel has its own suite
// next door. What is worth pinning here is that the two are actually joined up: a
// route that rendered its title and nothing else would look fine in a screenshot
// and be useless.
//
// Who may reach it is not tested here, because it is not this component's to
// decide. The route condition lives in App.svelte and the real check is the API's
// /admin subtree, so the gating is asserted in e2e/auth.spec.ts against a browser
// that can open the account menu, and in the Go suite against the endpoints.

const getActivityStatus = vi.hoisted(() => vi.fn());
const getActivitySchedule = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  getActivityStatus,
  getActivitySchedule,
}));

beforeEach(() => {
  vi.clearAllMocks();
  getActivityStatus.mockResolvedValue({
    status: 200,
    data: {
      running: false,
      lifters: 0,
      tickSeconds: 0,
      actions: 0,
      maxLifters: 8,
      maxWeeks: 26,
      roster: ["mara.quinn"],
    },
  });
  getActivitySchedule.mockResolvedValue({ status: 200, data: { enabled: false, lifters: 4 } });
});

describe("Astroturfing", () => {
  it("is titled for what it is", () => {
    render(Astroturfing);
    expect(screen.getByRole("heading", { name: "Astroturfing" })).toBeInTheDocument();
  });

  // The three modes, each of which the panel owns. Asserted through the route
  // because mounting the panel is the only thing this component does.
  it("mounts the controls", async () => {
    render(Astroturfing);

    // Waited on "Switch on" rather than on the first control: the others are static
    // markup and are there on the first paint, so a query for one of them would
    // pass before either request had landed and prove nothing.
    await waitFor(() => {
      expect(screen.getByText("Switch on")).toBeInTheDocument();
    });
    expect(screen.getByText("Generate history")).toBeInTheDocument();
    expect(screen.getByText("Start")).toBeInTheDocument();
    expect(screen.getByText("Remove generated lifters")).toBeInTheDocument();
  });
});
