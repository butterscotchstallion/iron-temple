/**
 * Whether the lifter has asked their system for less movement.
 *
 * Checked rather than assumed, and checked at each call rather than once: the
 * setting can change while the app is open, and on iOS it is a Control Centre
 * toggle people reach for mid-session. Nothing here is cached for that reason.
 *
 * Defaults to animating where the query cannot be asked — a browser too old for
 * matchMedia, or jsdom — because no answer is not the same as "yes".
 */
export function prefersReducedMotion(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return false;
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}
