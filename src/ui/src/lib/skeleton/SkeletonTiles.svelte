<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { cn } from "$lib/utils.js";
  import Skeleton from "./Skeleton.svelte";

  // The row of stat tiles — a small caption over a big number — that Racked, the
  // two recaps and another lifter's profile all open with.
  //
  // Structurally identical to the real thing on purpose: the same `<Card
  // class="p-4 text-center">` holding the same two children with the same `mt-1`
  // between them. That is what makes the swap free. A placeholder that merely
  // looked tile-shaped would have to guess at the Card's own flex gap — which
  // comes from `--card-spacing` and is not written at any of the call sites —
  // and would be wrong by 24px a tile the day that token moves.

  let {
    count = 4,
    value = "2xl",
    class: className,
  }: {
    /** How many tiles. Four on Racked and the recaps, two on a lifter's page. */
    count?: number;
    /**
     * Type size of the figure. `2xl` everywhere except the lifetime pair on a
     * lifter's profile, which sets them a step smaller — and a step is 4px of
     * page that would otherwise move.
     */
    value?: "xl" | "2xl";
    class?: string;
  } = $props();

  const tiles = $derived(Array.from({ length: count }, (_, i) => i));
</script>

<section class={cn("grid grid-cols-2 gap-3 sm:grid-cols-4", className)}>
  {#each tiles as i (i)}
    <Card class="flex flex-col items-center p-4 text-center">
      <Skeleton text="xs" class="w-16" />
      <Skeleton text={value} class="mt-1 w-20" />
    </Card>
  {/each}
</section>
