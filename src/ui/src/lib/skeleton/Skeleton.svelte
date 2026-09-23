<script lang="ts">
  import { cn } from "$lib/utils.js";

  // One grey bar standing in for one piece of content that hasn't arrived.
  //
  // The point of this component is the `text` prop, and it is worth being clear
  // about why, because "a pulsing rectangle" is otherwise three classes nobody
  // needs a component for.
  //
  // A skeleton only prevents a reflow if it is the SAME HEIGHT as the thing it
  // replaces. Every placeholder in this app used to be a fixed-height empty Card
  // — `h-40` standing in for a page, `h-24` for a card — which reserved a
  // plausible-looking amount of space and then jumped the moment real content
  // landed and turned out to be a different size. Matching exactly means matching
  // the line box a piece of text occupies, and Tailwind's type scale pins that:
  // `text-2xl` is a 2rem line box whatever the string in it says. So a bar
  // standing in for a `text-2xl` heading asks for `text="2xl"` and gets `h-8`,
  // and the two agree by construction rather than by someone having measured.
  //
  // The widths stay the caller's job. Height is what shifts the page; width only
  // decides how convincing the shimmer looks, and it varies per call site.

  /**
   * Tailwind's type scale, as the line-box height each size occupies.
   *
   * These are line heights, not font sizes: `text-sm` is a 0.875rem font in a
   * 1.25rem box, and 1.25rem (h-5) is what the surrounding layout reserves for
   * it. Getting this wrong by a step is a 4px jump per line, which on a table of
   * twenty rows is most of a screen.
   */
  const TEXT_HEIGHTS = {
    xs: "h-4",
    sm: "h-5",
    base: "h-6",
    lg: "h-7",
    xl: "h-7",
    "2xl": "h-8",
    "3xl": "h-9",
    "4xl": "h-10",
    "5xl": "h-12",
  } as const;

  type TextSize = keyof typeof TEXT_HEIGHTS;

  let {
    text,
    class: className,
    style,
  }: {
    /** Stand in for a line of text at this Tailwind size, matching its height. */
    text?: TextSize;
    /** Width, and any height for a block that isn't a line of text. */
    class?: string;
    /**
     * For a dimension Tailwind cannot see — an aspect ratio built from a
     * constant in a .ts file, say. Prefer a class wherever one exists.
     */
    style?: string;
  } = $props();
</script>

<!-- aria-hidden throughout: a screen reader gets one "Loading…" from the
     <Loading> region around these, and announcing a dozen empty boxes inside it
     would bury that. -->
<div
  class={cn(
    "animate-pulse rounded-md bg-muted/40 motion-reduce:animate-none",
    text && TEXT_HEIGHTS[text],
    className,
  )}
  {style}
  aria-hidden="true"
></div>
