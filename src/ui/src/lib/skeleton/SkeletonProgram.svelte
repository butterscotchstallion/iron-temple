<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import Skeleton from "./Skeleton.svelte";

  // The program screen, before it arrives: a name, a description, and a card per
  // workout day holding a heading, the weekday picker and the day's lifts.
  //
  // Shared by ProgramDetail and by Home, and the sharing is the point rather
  // than a saving. Home fetches its session list and THEN mounts ProgramDetail,
  // which fetches the program — two waits, back to back, with a placeholder
  // either side of the handover. Two different placeholders there would jump in
  // the middle of loading, which is the one reflow a skeleton has no excuse for.

  let {
    days = 3,
    lifts = 4,
  }: {
    /** Workout days in the program. Three or four, for everything shipped here. */
    days?: number;
    /** Prescribed lifts a day. */
    lifts?: number;
  } = $props();

  const dayList = $derived(Array.from({ length: days }, (_, i) => i));
  const liftList = $derived(Array.from({ length: lifts }, (_, i) => i));
</script>

<!-- The name and description. Neither the attribution line under them nor the
     Edit link beside them is reserved: both appear only on a program the lifter
     built, and the seeded ones — which is what most installs open on — have
     neither. -->
<div>
  <Skeleton text="3xl" class="w-64" />
  <Skeleton text="sm" class="mt-1 w-80" />
</div>

{#each dayList as day (day)}
  <Card class="p-5">
    <div class="flex items-center justify-between gap-3">
      <div class="flex items-center gap-2">
        <Skeleton text="lg" class="w-32" />
        <!-- The weekday picker: a bordered control at px-2 py-1 around text-xs,
             which is 26px however the day is spelled. -->
        <Skeleton class="h-[26px] w-28 rounded-md" />
      </div>
      <!-- Start. `size="sm"` buttons are h-8 here. -->
      <Skeleton class="h-8 w-20 shrink-0 rounded-md" />
    </div>
    <ul class="mt-3 flex flex-col gap-1.5">
      {#each liftList as lift (lift)}
        <li class="flex items-baseline justify-between gap-2">
          <Skeleton text="sm" class="w-36" />
          <Skeleton text="sm" class="w-28" />
        </li>
      {/each}
    </ul>
  </Card>
{/each}
