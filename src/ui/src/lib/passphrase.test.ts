import { describe, expect, it } from "vitest";
import { PASSPHRASE_WORDS, WORDS, passphrase } from "./passphrase";

// The wordlist's properties are the passphrase's security, so they are asserted
// rather than eyeballed. A duplicate word or a quiet prune costs entropy
// silently — nothing about the generated string looks any different afterwards.

describe("the wordlist", () => {
  it("has no duplicates", () => {
    const seen = new Map<string, number>();
    for (const word of WORDS) seen.set(word, (seen.get(word) ?? 0) + 1);
    const repeated = [...seen.entries()].filter(([, n]) => n > 1).map(([w]) => w);
    // A duplicate is not a typo, it is a word drawn twice as often as its
    // neighbours — so name them rather than just failing a count.
    expect(repeated).toEqual([]);
  });

  // The floor, not the exact size: adding words is always fine, and pinning the
  // count would make a useful addition look like a regression. Removing enough
  // to weaken the passphrase is what this catches.
  it("is large enough for the entropy the module claims", () => {
    expect(WORDS.length).toBeGreaterThanOrEqual(512);
    const bits = PASSPHRASE_WORDS * Math.log2(WORDS.length);
    expect(bits).toBeGreaterThanOrEqual(36);
  });

  it("is spellable from hearing: 3-9 lowercase letters, nothing else", () => {
    const offenders = WORDS.filter((w) => !/^[a-z]{3,9}$/.test(w));
    expect(offenders).toEqual([]);
  });
});

describe("passphrase", () => {
  it("joins four words with hyphens", () => {
    const parts = passphrase().split("-");
    expect(parts).toHaveLength(PASSPHRASE_WORDS);
    for (const part of parts) expect(WORDS).toContain(part);
  });

  // The API rejects anything under 8 characters, and a generated password that
  // its own server refuses would be a fine way to make this feature useless.
  it("always clears the API's length rules", () => {
    for (let i = 0; i < 200; i++) {
      const generated = passphrase();
      expect(generated.length).toBeGreaterThanOrEqual(8);
      // maxPasswordLen is 256 BYTES; the list is ASCII, so length is bytes.
      expect(generated.length).toBeLessThanOrEqual(256);
    }
  });

  it("does not repeat itself", () => {
    const generated = new Set(Array.from({ length: 500 }, () => passphrase()));
    // 500 draws from ~37 bits: a collision would mean the generator is not
    // drawing independently, not that we got unlucky.
    expect(generated.size).toBe(500);
  });

  // Weak randomness is the failure that looks fine in every screenshot, so the
  // spread is checked rather than assumed. Not a statistical test — it catches
  // a generator stuck on a constant, a prefix, or a handful of indices.
  it("draws across the whole list", () => {
    const seen = new Set<string>();
    for (let i = 0; i < 2000; i++) {
      for (const word of passphrase().split("-")) seen.add(word);
    }
    // 8000 draws over ~700 words: expected coverage is essentially total, so
    // half the list is a floor no working generator can miss.
    expect(seen.size).toBeGreaterThan(WORDS.length / 2);
  });

  it("honours a different word count", () => {
    expect(passphrase(6).split("-")).toHaveLength(6);
  });
});
