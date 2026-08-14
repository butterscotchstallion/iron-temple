import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import ErrorBoundaryFixture from "./ErrorBoundaryFixture.svelte";

describe("ErrorBoundary", () => {
  beforeEach(() => {
    // The boundary logs on purpose; keep the expected noise out of the run.
    vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders its children when nothing throws", () => {
    render(ErrorBoundaryFixture, { shouldThrow: false });
    expect(screen.getByText("the route rendered fine")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("shows the error page instead of a blank screen when a child throws", () => {
    render(ErrorBoundaryFixture, { shouldThrow: true });
    expect(screen.getByRole("alert")).toHaveTextContent("Something went wrong");
    expect(screen.queryByText("the route rendered fine")).toBeNull();
  });

  it("surfaces the failure to the console so it stays debuggable", () => {
    render(ErrorBoundaryFixture, { shouldThrow: true });
    expect(console.error).toHaveBeenCalledWith(
      "Unhandled render error:",
      expect.objectContaining({ message: "kaboom" }),
    );
  });
});
