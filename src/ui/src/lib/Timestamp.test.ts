import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "@testing-library/svelte";
import Timestamp from "./Timestamp.svelte";
import { resetClock } from "./now.svelte";

// The one date component: relative text on screen, the exact moment in the
// title. The ladder's own wording is covered in date.test.ts; this covers what
// only the component can get wrong — which formatter each `kind` reaches for,
// what it renders for an absent date, and whether it actually follows the clock.

// A fixed "now" so these say what they mean rather than depending on when they
// run, and local components throughout so they do not depend on where either.
const NOW = new Date(2026, 8, 24, 12, 0, 0);
const at = (
  year: number,
  month: number,
  day: number,
  hour = 12,
  minute = 0,
) => new Date(year, month, day, hour, minute, 0).toISOString();

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
  resetClock();
});

afterEach(() => {
  resetClock();
  vi.useRealTimers();
});

const time = (container: HTMLElement) => container.querySelector("time");

describe("Timestamp", () => {
  it("shows an instant as relative text with the exact moment in the title", () => {
    const { container } = render(Timestamp, {
      props: { value: at(2026, 8, 22, 9, 0) },
    });

    const el = time(container);
    expect(el).not.toBeNull();
    expect(el).toHaveTextContent("2 days ago");
    expect(el).toHaveAttribute("title", "Sep 22 2026 09:00AM");
  });

  it("carries the raw value in datetime, so it is machine-readable", () => {
    // The relative text is for a reader; the attribute is what makes the element
    // mean anything to something that is not one.
    const value = at(2026, 8, 22, 9, 0);
    const { container } = render(Timestamp, { props: { value } });

    expect(time(container)).toHaveAttribute("datetime", value);
  });

  it("gives a date-only value a title with no time of day", () => {
    // There is no time on the wire, so inventing midnight would report a
    // precision the server never sent.
    const { container } = render(Timestamp, {
      props: { value: "2026-09-22", kind: "date" as const },
    });

    const el = time(container);
    expect(el).toHaveTextContent("2 days ago");
    expect(el).toHaveAttribute("title", "September 22 2026");
  });

  it("reads a date-only value as its own day rather than as UTC midnight", () => {
    // The whole reason `kind` exists. Through the instant path this date would
    // parse as UTC midnight and read a day early west of Greenwich.
    const { container } = render(Timestamp, {
      props: { value: "2026-09-24", kind: "date" as const },
    });

    expect(time(container)).toHaveTextContent("today");
  });

  it("renders nothing at all when there is no date", () => {
    // Not "Invalid Date", and not an empty tooltip on an empty element. Callers
    // own the prose around it.
    const { container } = render(Timestamp, { props: { value: null } });
    expect(time(container)).toBeNull();

    const undef = render(Timestamp, { props: { value: undefined } });
    expect(time(undef.container)).toBeNull();
  });

  it("passes a class through, since call sites style this text", () => {
    const { container } = render(Timestamp, {
      props: { value: at(2026, 8, 22), class: "text-xs text-muted-foreground" },
    });

    expect(time(container)).toHaveClass("text-xs", "text-muted-foreground");
  });

  it("advances on its own as the clock moves", async () => {
    // The point of the shared ticker: a comment posted while the recap is open
    // must stop saying "just now" without anybody reloading.
    // Thirty seconds old, so it starts inside the "just now" window.
    const thirtySecondsAgo = new Date(NOW.getTime() - 30_000).toISOString();
    const { container } = render(Timestamp, {
      props: { value: thirtySecondsAgo },
    });
    expect(time(container)).toHaveTextContent("just now");

    await vi.advanceTimersByTimeAsync(60_000);

    expect(time(container)).toHaveTextContent("1 minute ago");
  });
});
