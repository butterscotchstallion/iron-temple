import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import ErrorBanner from "./ErrorBanner.svelte";

describe("ErrorBanner", () => {
  it("renders the message as an alert", () => {
    render(ErrorBanner, { message: "Something broke" });
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Something broke");
  });

  it("omits Retry and Dismiss when no handlers are given", () => {
    render(ErrorBanner, { message: "oops" });
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Dismiss" })).toBeNull();
  });

  it("calls onRetry when Retry is clicked", async () => {
    const onRetry = vi.fn();
    render(ErrorBanner, { message: "oops", onRetry });
    await fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("calls onDismiss when Dismiss is clicked", async () => {
    const onDismiss = vi.fn();
    render(ErrorBanner, { message: "oops", onDismiss });
    await fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(onDismiss).toHaveBeenCalledOnce();
  });
});
