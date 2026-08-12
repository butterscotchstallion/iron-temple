import { describe, it, expect } from "vitest";
import { cn } from "./utils";

describe("cn", () => {
  it("joins class names", () => {
    expect(cn("a", "b")).toBe("a b");
  });

  it("drops falsy values and resolves conditionals (clsx)", () => {
    expect(cn("a", false, null, undefined, "b")).toBe("a b");
    expect(cn("base", { active: true, hidden: false })).toBe("base active");
    expect(cn(["a", ["b", "c"]])).toBe("a b c");
  });

  it("merges conflicting Tailwind utilities, last one winning (tailwind-merge)", () => {
    expect(cn("px-2 px-4")).toBe("px-4");
    expect(cn("text-sm", "text-lg")).toBe("text-lg");
    // Non-conflicting utilities are both kept.
    expect(cn("p-2 text-red-500")).toBe("p-2 text-red-500");
  });

  it("returns an empty string for no meaningful input", () => {
    expect(cn()).toBe("");
    expect(cn(false, null, undefined)).toBe("");
  });
});
