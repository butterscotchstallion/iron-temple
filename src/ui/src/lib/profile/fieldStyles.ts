/**
 * The input and label styling the profile's forms share.
 *
 * Lifted out of Profile.svelte when it was split into sections: the two strings
 * were a pair of local consts used by every form on the page, and copying them
 * into each section would have been four chances for the fields to drift apart
 * on a screen whose whole job is to look like one screen.
 *
 * Deliberately scoped to the profile rather than promoted to a shared UI
 * primitive — other screens spell the same classes out for themselves, and
 * unifying those is a bigger change than this one.
 */
export const fieldClass =
  "rounded-md border border-border/60 bg-input/40 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary";

export const labelClass =
  "text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground";
