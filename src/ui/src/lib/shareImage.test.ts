import { afterEach, describe, expect, it, vi } from "vitest";
import type { RackedPeriod } from "./api";
import { canShareFile, shareCardFile, shareCardFilename, shareOrDownload } from "./shareImage";

// The handoff, tested without a canvas.
//
// renderShareCard is not covered here and cannot be: jsdom returns null from
// getContext("2d"), so every path through it ends at the guard clause. What it
// does beyond that guard — paint, encode — is shareCard.test.ts's job and the
// browser's. What is covered is the part with branches in it: which of the two
// ways a browser will take a file, and what a cancelled share sheet means.

function period(label: string): RackedPeriod {
  return { kind: "month", start: "2026-03-01", end: "2026-03-31", label, inProgress: false };
}

function file(): File {
  return shareCardFile(new Blob(["png"], { type: "image/png" }), "racked-march-2026.png");
}

/**
 * Replace navigator's sharing surface for one test. Spread over the real
 * navigator rather than replacing it, so anything else reading it still works.
 */
function withNavigator(props: {
  share?: Navigator["share"];
  canShare?: Navigator["canShare"];
}) {
  vi.stubGlobal("navigator", { ...globalThis.navigator, ...props });
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("shareCardFilename", () => {
  it("names the file after the period", () => {
    expect(shareCardFilename(period("March 2026"))).toBe("racked-march-2026.png");
    expect(shareCardFilename(period("2026"))).toBe("racked-2026.png");
  });

  // The label comes from the server and is the only part of the name that is
  // not a literal; a separator that leaked through would make a filename a
  // path.
  it("reduces anything else in the label to single dashes", () => {
    expect(shareCardFilename(period("March / 2026"))).toBe("racked-march-2026.png");
    expect(shareCardFilename(period("  March  2026  "))).toBe("racked-march-2026.png");
    expect(shareCardFilename(period("../../etc"))).toBe("racked-etc.png");
  });

  it("still produces a usable name from a label with nothing in it", () => {
    expect(shareCardFilename(period(""))).toBe("racked-recap.png");
  });
});

describe("canShareFile", () => {
  it("is false where the browser cannot share files at all", () => {
    withNavigator({ share: undefined, canShare: undefined });
    expect(canShareFile(file())).toBe(false);
  });

  // Desktop Chrome has navigator.share for text but refuses image files.
  it("is false where the browser shares, but not this file", () => {
    withNavigator({ share: vi.fn(), canShare: () => false });
    expect(canShareFile(file())).toBe(false);
  });

  it("is true where the browser will take the image", () => {
    withNavigator({ share: vi.fn(), canShare: () => true });
    expect(canShareFile(file())).toBe(true);
  });
});

describe("shareOrDownload", () => {
  it("hands the file to the share sheet where there is one", async () => {
    const share = vi.fn().mockResolvedValue(undefined);
    withNavigator({ share, canShare: () => true });

    const png = file();
    await expect(shareOrDownload(png)).resolves.toBe("shared");
    expect(share).toHaveBeenCalledWith({ files: [png], title: "Racked" });
  });

  // Backing out of the share sheet is a decision, not a failure. Reporting it
  // as one would put an error under the preview every time somebody changed
  // their mind.
  it("reports a dismissed share sheet as a cancel", async () => {
    const abort = new Error("share cancelled");
    abort.name = "AbortError";
    withNavigator({ share: vi.fn().mockRejectedValue(abort), canShare: () => true });

    await expect(shareOrDownload(file())).resolves.toBe("cancelled");
  });

  it("lets a genuine share failure through to the caller", async () => {
    const boom = new Error("share target exploded");
    withNavigator({ share: vi.fn().mockRejectedValue(boom), canShare: () => true });

    await expect(shareOrDownload(file())).rejects.toThrow("share target exploded");
  });

  it("saves the file where the browser has no share sheet", async () => {
    withNavigator({ share: undefined, canShare: undefined });

    const createObjectURL = vi.fn().mockReturnValue("blob:racked");
    vi.stubGlobal("URL", { ...URL, createObjectURL, revokeObjectURL: vi.fn() });

    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

    await expect(shareOrDownload(file())).resolves.toBe("downloaded");
    expect(createObjectURL).toHaveBeenCalled();
    expect(click).toHaveBeenCalled();
  });

  // Revoking as soon as click() returns saves an empty file in Safari, which
  // has not finished reading the URL by then.
  it("holds the object URL open past the click", async () => {
    vi.useFakeTimers();
    try {
      withNavigator({ share: undefined, canShare: undefined });

      const revokeObjectURL = vi.fn();
      vi.stubGlobal("URL", {
        ...URL,
        createObjectURL: vi.fn().mockReturnValue("blob:racked"),
        revokeObjectURL,
      });
      vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

      await shareOrDownload(file());
      expect(revokeObjectURL).not.toHaveBeenCalled();

      vi.runAllTimers();
      expect(revokeObjectURL).toHaveBeenCalledWith("blob:racked");
    } finally {
      vi.useRealTimers();
    }
  });
});
