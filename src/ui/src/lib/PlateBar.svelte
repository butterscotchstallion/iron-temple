<script lang="ts">
  import { platesPerSide } from "./plates";

  let { weightLb }: { weightLb: number } = $props();

  // Plates for one side, largest nearest the collar.
  const perSide = $derived(platesPerSide(weightLb));
  // Left half mirrors the right, outermost (smallest) first.
  const leftSide = $derived([...perSide].reverse());

  // Plate disc height in px, scaled by weight (45 tallest, 2.5 shortest).
  function plateHeight(plate: number): number {
    return Math.round(24 + (plate / 45) * 40);
  }
</script>

<div
  class="flex items-center justify-center gap-[3px]"
  aria-label={`Barbell loaded to ${weightLb} lb`}
>
  {#if perSide.length === 0}
    <div class="h-1.5 w-24 rounded-full bg-muted-foreground/40"></div>
    <span class="ml-2 text-xs text-muted-foreground">just the bar</span>
  {:else}
    {#each leftSide as plate, i (i)}
      <div
        class="flex w-5 items-center justify-center rounded-[3px] bg-primary text-[9px] font-bold tabular-nums text-primary-foreground"
        style="height: {plateHeight(plate)}px"
        title={`${plate} lb`}
      >
        {plate}
      </div>
    {/each}
    <div class="mx-1 h-1.5 w-8 rounded-full bg-muted-foreground/50"></div>
    {#each perSide as plate, i (i)}
      <div
        class="flex w-5 items-center justify-center rounded-[3px] bg-primary text-[9px] font-bold tabular-nums text-primary-foreground"
        style="height: {plateHeight(plate)}px"
        title={`${plate} lb`}
      >
        {plate}
      </div>
    {/each}
  {/if}
</div>
