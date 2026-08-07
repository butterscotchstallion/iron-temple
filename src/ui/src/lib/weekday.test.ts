import { describe, it, expect } from "vitest";
import { weekdayLabel } from "./weekday";

describe("weekdayLabel", () => {
  it("names weekdays 0..6", () => {
    expect(weekdayLabel(0)).toBe("Sunday");
    expect(weekdayLabel(2)).toBe("Tuesday");
    expect(weekdayLabel(6)).toBe("Saturday");
  });

  it("is Unscheduled for null/undefined", () => {
    expect(weekdayLabel(null)).toBe("Unscheduled");
    expect(weekdayLabel(undefined)).toBe("Unscheduled");
  });
});
