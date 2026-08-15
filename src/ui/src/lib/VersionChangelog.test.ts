import { describe, it, expect } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import VersionChangelog from "./VersionChangelog.svelte";

// The component's notes normally come from virtual:iron-temple/changelog, which
// vite.config.ts fills from git at build time. Tests pass them in explicitly
// instead, so what is asserted here doesn't change with the checkout's history.
const notes = {
  version: "v1.2.3",
  entries: [
    "feat(ui): show the changelog when you hover the version (abc1234)",
    "fix(api): stop 500ing on an empty program (def5678)",
  ],
};

const empty = { version: "", entries: [] };

describe("VersionChangelog", () => {
  it("renders the version and environment reported by the API", () => {
    render(VersionChangelog, {
      props: { version: "v1.2.3", environment: "production", notes },
    });
    expect(screen.getByTestId("version")).toHaveTextContent("iron-temple v1.2.3-production");
  });

  it("omits the environment suffix when the API doesn't report one", () => {
    render(VersionChangelog, { props: { version: "v1.2.3", environment: "", notes } });
    expect(screen.getByTestId("version")).toHaveTextContent("iron-temple v1.2.3");
  });

  // A build with no notes — a dev checkout, or a release of nothing but chores —
  // must leave the header exactly as it was before this feature: plain text, with
  // nothing to suggest there is something to open.
  it("renders inert text when the build shipped no release notes", () => {
    render(VersionChangelog, {
      props: { version: "v1.2.3", environment: "production", notes: empty },
    });
    expect(screen.getByTestId("version").tagName).toBe("SPAN");
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("renders nothing when /health hasn't reported a version", () => {
    render(VersionChangelog, { props: { version: "", environment: "", notes } });
    expect(screen.getByTestId("version")).toHaveTextContent("");
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  // A button, not LinkPreview's default anchor: it navigates nowhere, and a tap or
  // Enter has to reach it on a touch screen, where the hover handlers bail out.
  it("makes the version a button when there are notes to show", () => {
    render(VersionChangelog, {
      props: { version: "v1.2.3", environment: "production", notes },
    });
    const trigger = screen.getByTestId("version");
    expect(trigger.tagName).toBe("BUTTON");
    // The version is the whole of the button's name and text — what it does is a
    // separate description. Nesting the hint inside would append it to the text
    // content, which e2e/auth.spec.ts asserts on exactly.
    expect(trigger).toHaveTextContent(/^iron-temple v1\.2\.3-production$/);
    expect(trigger).toHaveAccessibleName("iron-temple v1.2.3-production");
    expect(trigger).toHaveAccessibleDescription("Show the release notes for this version");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
  });

  it("opens the panel on click and lists the release notes", async () => {
    render(VersionChangelog, {
      props: { version: "v1.2.3", environment: "production", notes },
    });
    await fireEvent.click(screen.getByTestId("version"));

    const panel = await screen.findByTestId("changelog-panel");
    // Titled from the notes' own version, never the one /health reported, so the
    // panel can't label notes as belonging to a release they didn't come from.
    expect(panel).toHaveTextContent("What's new in v1.2.3");
    for (const entry of notes.entries) expect(panel).toHaveTextContent(entry);
    expect(screen.getByTestId("version")).toHaveAttribute("aria-expanded", "true");

    // Close before the test ends. bits-ui's LinkPreview logs a Svelte
    // derived_inert warning when it is unmounted with the panel still open
    // (reproducible with the primitive alone, nothing to do with this
    // component), and the rest of the suite runs warning-free — worth keeping it
    // that way so a real one stands out.
    await fireEvent.click(screen.getByTestId("version"));
  });

  // The same click that opens it on a touch screen has to close it again —
  // there is no pointer to move away, and Escape needs a keyboard.
  it("closes the panel when the trigger is clicked again", async () => {
    render(VersionChangelog, {
      props: { version: "v1.2.3", environment: "production", notes },
    });
    await fireEvent.click(screen.getByTestId("version"));
    await screen.findByTestId("changelog-panel");

    await fireEvent.click(screen.getByTestId("version"));
    await waitFor(() =>
      expect(screen.queryByTestId("changelog-panel")).not.toBeInTheDocument(),
    );
  });
});
