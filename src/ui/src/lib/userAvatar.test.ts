import { describe, it, expect } from "vitest";
import { avatarColor, avatarUrl, initials } from "./userAvatar";

describe("avatarColor", () => {
  it("uses the colour the user chose", () => {
    expect(avatarColor(1, "#123456")).toBe("#123456");
  });

  it("derives one from the id when none is chosen", () => {
    expect(avatarColor(1, "")).toMatch(/^#[0-9a-f]{6}$/i);
  });

  // The same person must be the same colour on every render and every device,
  // which rules out anything random.
  it("is stable for a given id", () => {
    expect(avatarColor(7, "")).toBe(avatarColor(7, ""));
  });

  it("spreads different ids across the palette", () => {
    const colours = new Set([0, 1, 2, 3, 4].map((id) => avatarColor(id, "")));
    expect(colours.size).toBeGreaterThan(1);
  });

  it("handles a negative or non-integer id without falling off the palette", () => {
    for (const id of [-1, -12, 3.7]) {
      expect(avatarColor(id, "")).toMatch(/^#[0-9a-f]{6}$/i);
    }
  });
});

describe("initials", () => {
  it("takes the first letter of the first and last words", () => {
    expect(initials("Ada Lovelace")).toBe("AL");
    expect(initials("Ada Byron King Lovelace")).toBe("AL");
  });

  it("takes one letter from a single word", () => {
    expect(initials("ada")).toBe("A");
  });

  it("ignores surrounding and repeated whitespace", () => {
    expect(initials("  Ada   Lovelace  ")).toBe("AL");
  });

  // A chip with no letters in it reads as a broken image, so an empty name
  // still has to produce something.
  it("falls back to a placeholder for an empty name", () => {
    expect(initials("")).toBe("?");
    expect(initials("   ")).toBe("?");
  });

  it("does not split a non-BMP character in half", () => {
    // Naive charAt(0) would return half a surrogate pair here.
    expect(initials("🏋️ Lifter")).toBe([..."🏋️"][0].toUpperCase() + "L");
    expect([...initials("𝔄da")][0]).toBe("𝔄");
  });
});

describe("avatarUrl", () => {
  it("points at the user's avatar endpoint", () => {
    expect(avatarUrl(42, undefined)).toBe("/api/v1/users/42/avatar");
  });

  // Without the cache-buster a fresh upload would keep showing the old image
  // until the cache revalidated.
  it("appends the etag so a new upload busts the cache", () => {
    expect(avatarUrl(42, "abc123")).toBe("/api/v1/users/42/avatar?v=abc123");
  });

  it("escapes an etag that would otherwise break the query string", () => {
    expect(avatarUrl(42, 'a"b&c')).toBe("/api/v1/users/42/avatar?v=a%22b%26c");
  });
});
