import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import ErrorPage from "./ErrorPage.svelte";

describe("ErrorPage", () => {
  afterEach(() => {
    window.location.hash = "";
  });

  it("announces the failure as an alert", () => {
    render(ErrorPage, {});
    expect(screen.getByRole("alert")).toHaveTextContent("Something went wrong");
  });

  it("shows the gym fire image", () => {
    render(ErrorPage, {});
    const img = screen.getByAltText("A gym engulfed in flames");
    expect(img).toHaveAttribute("src", "/images/gym-fire.png");
  });

  it("drops the image rather than leaving a broken one when it fails to load", async () => {
    render(ErrorPage, {});
    await fireEvent.error(screen.getByAltText("A gym engulfed in flames"));
    expect(screen.queryByAltText("A gym engulfed in flames")).toBeNull();
    // The page itself must survive the missing asset.
    expect(screen.getByRole("alert")).toHaveTextContent("Something went wrong");
  });

  it("shows the message of a thrown Error", () => {
    render(ErrorPage, { error: new Error("boom") });
    expect(screen.getByRole("alert")).toHaveTextContent("boom");
  });

  it("shows nothing extra when the thrown value has no message", () => {
    render(ErrorPage, { error: { weird: true } });
    expect(screen.getByRole("alert")).not.toHaveTextContent("object Object");
  });

  it("has no Try again button without a reset handler", () => {
    render(ErrorPage, {});
    expect(screen.queryByRole("button", { name: "Try again" })).toBeNull();
  });

  it("calls onReset when Try again is clicked", async () => {
    const onReset = vi.fn();
    render(ErrorPage, { onReset });
    await fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(onReset).toHaveBeenCalledOnce();
  });

  it("navigates home and resets, so home isn't rendered as the failed route", async () => {
    const onReset = vi.fn();
    window.location.hash = "#/history";
    render(ErrorPage, { onReset });
    await fireEvent.click(screen.getByRole("button", { name: "Go home" }));
    expect(window.location.hash).toBe("#/");
    expect(onReset).toHaveBeenCalledOnce();
  });
});
