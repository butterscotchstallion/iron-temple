import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import Avatar from "./Avatar.svelte";
import type { User } from "./api";

function user(overrides: Partial<User> = {}): User {
  return {
    id: 1,
    username: "ada",
    displayName: "Ada Lovelace",
    avatarColor: "",
    isAdmin: true,
    hasAvatar: false,
    ...overrides,
  };
}

describe("Avatar", () => {
  it("renders the initials chip when there is no uploaded image", () => {
    render(Avatar, { props: { user: user() } });
    expect(screen.getByText("AL")).toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });

  it("renders the image when one has been uploaded", () => {
    render(Avatar, {
      props: { user: user({ hasAvatar: true, avatarEtag: "etag1" }) },
    });
    const img = document.querySelector("img");
    expect(img).not.toBeNull();
    expect(img?.getAttribute("src")).toBe("/api/v1/users/1/avatar?v=etag1");
    expect(screen.queryByText("AL")).not.toBeInTheDocument();
  });

  it("uses the chosen chip colour", () => {
    render(Avatar, { props: { user: user({ avatarColor: "#123456" }) } });
    expect(screen.getByText("AL")).toHaveStyle({ backgroundColor: "#123456" });
  });

  it("falls back to the username when there is no display name", () => {
    render(Avatar, { props: { user: user({ displayName: "" }) } });
    expect(screen.getByText("A")).toBeInTheDocument();
  });

  it("applies the requested size", () => {
    render(Avatar, { props: { user: user(), size: 64 } });
    expect(screen.getByText("AL")).toHaveStyle({ width: "64px", height: "64px" });
  });

  // The image is decorative — the display name is always rendered beside it —
  // so it must not be announced twice.
  it("does not expose the chip to the accessibility tree", () => {
    render(Avatar, { props: { user: user() } });
    expect(screen.getByText("AL")).toHaveAttribute("aria-hidden", "true");
  });
});
