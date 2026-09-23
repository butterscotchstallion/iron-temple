<script lang="ts">
  import { cn } from "$lib/utils.js";
  import Skeleton from "./Skeleton.svelte";

  // A divided list of "somebody, and two lines about them" rows.
  //
  // Five surfaces draw that list against real data — the feed, the leaderboard,
  // the lifter roster, the account roster, the comments under a recap — and they
  // agree on its geometry down to the divider: an avatar, a name, a line of
  // detail, `py` of about three. So they share one placeholder rather than five
  // near-copies, which is also the only way the five stay in step when the row
  // padding next changes.
  //
  // The avatar arrives as a size CLASS rather than a number of pixels, because
  // that is the one dimension the five differ on and it is what sets the row
  // height — a `size-10` chip and a `size-7` chip are 12px a row, which down a
  // full list is a visible jump. A class rather than a number so it stays in the
  // same vocabulary as the `<Avatar>` it stands in for, and so nothing here has
  // to reach for an inline style Tailwind cannot see.

  let {
    rows = 4,
    avatar = "size-9",
    trailing = false,
    rowClass = "py-3",
    class: className,
  }: {
    /** How many to draw. Match what the endpoint usually returns, not its max. */
    rows?: number;
    /** Tailwind size class for the avatar chip, or null for a list without one. */
    avatar?: string | null;
    /** A right-aligned figure — a rank's score, a session's set count. */
    trailing?: boolean;
    /** The row's own padding. The leaderboard packs its rows tighter than the feed. */
    rowClass?: string;
    class?: string;
  } = $props();

  // Varied so the stack reads as a list of different people rather than as a
  // form. The two cycles are different lengths, so the name and the detail line
  // of a row don't march in step down the page either.
  const NAME_WIDTHS = ["w-32", "w-24", "w-40", "w-28"];
  const DETAIL_WIDTHS = ["w-44", "w-36", "w-52"];

  const items = $derived(Array.from({ length: rows }, (_, i) => i));
</script>

<ul class={cn("flex flex-col divide-y divide-border/60", className)}>
  {#each items as i (i)}
    <li class={cn("flex items-center gap-3", rowClass)}>
      {#if avatar}
        <Skeleton class={cn("shrink-0 rounded-full", avatar)} />
      {/if}
      <!-- No gap between the two bars, because the real lists stack their name
           and detail spans with none either. Four pixels a row is invisible on
           one and half a row's height down a list of ten. -->
      <div class="flex min-w-0 flex-1 flex-col">
        <Skeleton text="sm" class={NAME_WIDTHS[i % NAME_WIDTHS.length]} />
        <Skeleton text="xs" class={DETAIL_WIDTHS[i % DETAIL_WIDTHS.length]} />
      </div>
      {#if trailing}
        <Skeleton text="sm" class="w-14 shrink-0" />
      {/if}
    </li>
  {/each}
</ul>
