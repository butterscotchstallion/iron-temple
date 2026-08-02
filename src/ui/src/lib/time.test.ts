import { describe, it, expect } from "vitest";
import { formatTime } from "./time";

describe("formatTime", () => {
  it("formats minutes and seconds", () => {
    expect(formatTime(180)).toBe("3:00");
    expect(formatTime(65)).toBe("1:05");
    expect(formatTime(9)).toBe("0:09");
    expect(formatTime(0)).toBe("0:00");
  });

  it("clamps negatives to zero", () => {
    expect(formatTime(-5)).toBe("0:00");
  });

  it("floors fractional seconds", () => {
    expect(formatTime(59.9)).toBe("0:59");
  });
});
