import type { Options as ConfettiOptions } from "canvas-confetti";

/**
 * Confetti, on the two occasions the app has to celebrate: a set that beats a
 * record, and the recap of a finished workout.
 *
 * canvas-confetti is physics that only runs when something goes right, so it is
 * fetched at the first celebration rather than carried in either route's chunk.
 * The promise is cached at module scope, so a session full of PRs imports it
 * once — and so the recap, arriving after a PR has already fired, pays nothing.
 */
type ConfettiFn = (options?: ConfettiOptions) => unknown;

let loader: Promise<ConfettiFn> | undefined;

/**
 * Fire the confetti, loading it if this is the first time.
 *
 * Deliberately not awaited by callers: the celebration must never sit in front
 * of finishing a set. A failed chunk fetch is swallowed for the same reason —
 * losing the confetti is not losing the PR.
 *
 * The cast reconciles a mismatch in the package itself: canvas-confetti's types
 * are written for its CJS entry (`export = confetti`, so TypeScript types the
 * dynamic import as the bare callable), while the bundler resolves its ESM
 * build, which has a real default export. `.default` is what is actually there
 * at runtime.
 */
export function celebrate(options: ConfettiOptions): void {
  const fired = (loader ??= import("canvas-confetti").then(
    (m) => (m as unknown as { default: ConfettiFn }).default,
  ));
  void fired.then((fire) => fire(options)).catch(() => {});
}
