import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import ChangelogList from "./ChangelogList.svelte";

// The list shared by the header's changelog panel and the update prompt. Those
// two have their own specs for when notes appear; this one covers what neither
// asserts — that the list itself is sound however it is reached.

const entries = [
  "feat(ui): show the release notes in the update prompt (abc1234)",
  "fix(api): stop 500ing on an empty program (def5678)",
];

describe("ChangelogList", () => {
  it("renders one item per entry, in order", () => {
    render(ChangelogList, { props: { entries } });

    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(items[0]).toHaveTextContent(entries[0]);
    expect(items[1]).toHaveTextContent(entries[1]);
  });

  // The chevron is a bullet drawn in text. Without aria-hidden a screen reader
  // reads "rsaquo" (or the glyph) before every single entry, which turns a short
  // list of what shipped into a chore to listen to.
  it("hides the bullet from assistive tech", () => {
    render(ChangelogList, { props: { entries: [entries[0]] } });

    const item = screen.getByRole("listitem");
    expect(item.querySelectorAll("[aria-hidden='true']")).toHaveLength(1);
    // The entry itself is not what got hidden.
    expect(item).toHaveTextContent(entries[0]);
  });

  it("renders an empty list rather than failing when there are no entries", () => {
    render(ChangelogList, { props: { entries: [] } });

    // The <ul> is still there (svelte leaves its own anchor comments inside it),
    // but nothing is listed. Both callers hide the surrounding block in this
    // case, so this is only about not throwing on the way there.
    expect(screen.getByRole("list")).toBeInTheDocument();
    expect(screen.queryAllByRole("listitem")).toHaveLength(0);
  });

  // Both callers position the list themselves — the header panel wants a top
  // margin, the dialog gets its spacing from the alert dialog's grid gap.
  it("takes spacing from the caller", () => {
    render(ChangelogList, { props: { entries, class: "mt-3" } });

    expect(screen.getByRole("list")).toHaveClass("mt-3");
  });
});
