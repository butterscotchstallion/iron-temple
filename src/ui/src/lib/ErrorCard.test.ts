import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import ErrorCard from "./ErrorCard.svelte";

describe("ErrorCard", () => {
  it("renders the message as an alert", () => {
    render(ErrorCard, { message: "Couldn't load history" });
    expect(screen.getByRole("alert")).toHaveTextContent("Couldn't load history");
  });

  it("has no Retry button without a handler", () => {
    render(ErrorCard, { message: "nope" });
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });

  it("calls onRetry when Retry is clicked", async () => {
    const onRetry = vi.fn();
    render(ErrorCard, { message: "nope", onRetry });
    await fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });
});
