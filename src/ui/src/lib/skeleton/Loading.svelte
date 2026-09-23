<script lang="ts">
  import type { Snippet } from "svelte";
  import { cn } from "$lib/utils.js";

  // The wrapper every skeleton goes inside, and the half of "loading indicator"
  // that isn't visual.
  //
  // A shimmer says "wait" to somebody looking at the screen and says nothing at
  // all to somebody listening to it: the bars are aria-hidden (they have no text
  // to read), so a screen reader on a loading page used to find an empty
  // document and move on. This puts one polite live region around each stack of
  // bars with a sentence naming what is being fetched, so the page announces
  // "Loading your history…" once and then announces the content when it swaps in.
  //
  // `aria-busy` as well as the label, because the two answer different questions:
  // the label is what a screen reader says on arrival, and aria-busy is what
  // assistive tech checks before deciding a region is worth reading at all.
  //
  // The class is the caller's because a skeleton has to sit in the same box its
  // content will: a route whose loaded branch is a `flex flex-col gap-6` wants
  // the same here, and one whose placeholders are grid cells passes the grid.

  let {
    label,
    class: className,
    children,
  }: {
    /**
     * What is being loaded, as a sentence — "Loading your history". Named per
     * call site rather than defaulted to "Loading", because a lifter who has
     * just tapped between two tabs is told which one answered.
     */
    label: string;
    /** The layout the skeletons sit in; mirrors the loaded branch's own. */
    class?: string;
    children: Snippet;
  } = $props();
</script>

<div
  class={cn("flex flex-col gap-4", className)}
  role="status"
  aria-busy="true"
  data-testid="loading"
>
  <span class="sr-only">{label}…</span>
  {@render children()}
</div>
