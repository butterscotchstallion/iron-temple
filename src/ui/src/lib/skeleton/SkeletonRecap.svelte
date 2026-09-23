<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { cn } from "$lib/utils.js";
  import Skeleton from "./Skeleton.svelte";
  import SkeletonTiles from "./SkeletonTiles.svelte";

  // The recap, before it arrives.
  //
  // Shared by both recap screens — your own session and another lifter's —
  // because they draw the same three things in the same order: a two-line
  // heading, the four hero tiles, and the lift-by-lift table. What they disagree
  // on is whether the heading is centred, which is a `text-align` and not a
  // second component.
  //
  // The sections between those three (comparisons, highlights, the muscle bars)
  // are deliberately NOT reserved. Every one of them renders nothing when the
  // session has nothing to say — no PR, no milestone, a streak below the display
  // threshold — so holding space for them would be guessing at a layout that is
  // usually shorter, and guessing long is the same reflow in the other
  // direction.

  let {
    centered = false,
    lifts = 4,
  }: {
    /** Centre the heading, as another lifter's recap does. */
    centered?: boolean;
    /** Rows in the lift table. A program day here runs to three or four. */
    lifts?: number;
  } = $props();

  const rows = $derived(Array.from({ length: lifts }, (_, i) => i));
</script>

<div class={cn("flex flex-col gap-1", centered && "items-center")}>
  <Skeleton text="2xl" class="w-56" />
  <Skeleton text="sm" class="w-72" />
</div>

<SkeletonTiles count={4} />

<Card class="p-4">
  <Skeleton text="xs" class="w-20" />
  <ul class="mt-2 divide-y divide-border">
    {#each rows as row (row)}
      <li class="flex items-baseline gap-3 py-2">
        <div class="min-w-0 flex-1">
          <Skeleton text="sm" class="w-40" />
          <Skeleton text="xs" class="mt-0.5 w-32" />
        </div>
        <Skeleton text="sm" class="w-16 shrink-0" />
      </li>
    {/each}
  </ul>
</Card>
