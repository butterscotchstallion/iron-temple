import { router } from "svelte-spa-router";

/**
 * Which comment the current URL points at, or null.
 *
 * `?comment=<id>` is written by the notification panel when a lifter follows a
 * `comment` or `reply` row, and read by the two recap routes, which hand it to
 * SessionSocial. It exists because the recap a notification points at is long
 * and the conversation is the last card on it — arriving at the top to scroll
 * and hunt for a sentence you were just shown is not being taken anywhere.
 *
 * THE FIRST QUERY PARAMETER THIS APP READS. Everything else routes on path
 * segments, which is right for state that names a screen; this names a place
 * WITHIN one, survives a reload, and is absent on the ordinary visit — which is
 * what a query parameter is for. Not a hash fragment, because the router has
 * already spent the hash on the route itself.
 *
 * Deliberately strict. A value that is not a positive integer is treated as
 * absent rather than passed on to become a lookup for a comment that cannot
 * exist, so a hand-edited or truncated URL simply opens the recap.
 *
 * Reactive: `router.querystring` is the router's own state, so a caller that
 * reads this inside `$derived` re-reads it when the URL changes — which is what
 * makes following a second notification from the same recap work.
 */
export function markedCommentId(): number | null {
  const raw = new URLSearchParams(router.querystring ?? "").get("comment");
  if (raw === null) return null;

  const id = Number(raw);
  return Number.isInteger(id) && id > 0 ? id : null;
}
