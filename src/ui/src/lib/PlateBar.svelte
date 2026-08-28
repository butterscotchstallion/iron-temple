<script lang="ts">
  import { loadBar } from "./plates";
  import { barWeightLb, plateInventory } from "./gym.svelte";

  let { weightLb }: { weightLb: number } = $props();

  // Loaded against this lifter's own bar and rack, so the discs drawn here are
  // ones they can actually put on. A target the rack cannot build comes back
  // rounded down, and `loaded.weightLb` is what it rounded to.
  const loaded = $derived(loadBar(weightLb, barWeightLb(), plateInventory()));
  // Plates for one side, largest nearest the collar.
  const perSide = $derived(loaded.plates);
  // Left half mirrors the right, outermost (smallest) first.
  const leftSide = $derived([...perSide].reverse());

  // Plate disc height in px, scaled by weight (45 tallest, 2.5 shortest).
  function plateHeight(plate: number): number {
    return Math.round(24 + (plate / 45) * 40);
  }

  const barClass = "h-1.5 rounded-full bg-muted-foreground/50";
  const plateClass =
    "flex w-5 items-center justify-center rounded-[3px] bg-primary text-[9px] font-bold tabular-nums text-primary-foreground";
</script>

<div
  class="flex items-center justify-center gap-[3px]"
  aria-label={loaded.rounded
    ? `Barbell loaded to ${loaded.weightLb} lb — the closest this rack builds to ${weightLb}`
    : `Barbell loaded to ${weightLb} lb`}
>
  {#if perSide.length === 0}
    <div class="{barClass} w-40"></div>
    <span class="ml-2 text-xs text-muted-foreground">just the bar</span>
  {:else}
    <!-- outer sleeve end -->
    <div class="{barClass} w-4"></div>
    {#each leftSide as plate, i (i)}
      <div class={plateClass} style="height: {plateHeight(plate)}px" title={`${plate} lb`}>
        {plate}
      </div>
    {/each}
    <!-- shaft -->
    <div class="{barClass} mx-1 w-24"></div>
    {#each perSide as plate, i (i)}
      <div class={plateClass} style="height: {plateHeight(plate)}px" title={`${plate} lb`}>
        {plate}
      </div>
    {/each}
    <!-- outer sleeve end -->
    <div class="{barClass} w-4"></div>
  {/if}
</div>
