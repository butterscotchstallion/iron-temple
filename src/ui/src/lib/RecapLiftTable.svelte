<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import Check from "@lucide/svelte/icons/check";
  import { formatVolume } from "./volume";
  import { formatDelta } from "./racked";
  import { formatOutOf, type RecapLiftRow } from "./recap";

  // The workout, lift by lift, in the order it was prescribed — so the recap
  // can be read against the session the lifter has just come off, rather than
  // against an alphabetical list of the same movements.

  let { lifts }: { lifts: RecapLiftRow[] } = $props();
</script>

{#if lifts.length > 0}
  <Card class="p-4" data-testid="recap-lifts">
    <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">Every lift</h3>
    <ul class="mt-2 divide-y divide-border">
      {#each lifts as lift (lift.exerciseId)}
        <li class="flex items-baseline gap-3 py-2">
          <div class="min-w-0 flex-1">
            <p class="truncate font-bold text-foreground">
              {lift.exerciseName}
              {#if lift.kind === "assistance"}
                <span class="text-xs font-normal text-muted-foreground">assistance</span>
              {/if}
            </p>
            <p class="text-xs tabular-nums text-muted-foreground">
              {formatOutOf(lift.repsLogged, lift.repsTargeted)} reps · {formatOutOf(
                lift.setsLogged,
                lift.setsPrescribed,
              )} sets
              {#if lift.hitEveryTarget}
                <Check class="inline size-3 text-primary" aria-label="every set completed" />
              {/if}
            </p>
          </div>

          <div class="shrink-0 text-right">
            <p class="tabular-nums text-foreground">
              {formatVolume(lift.topWeightLb)} lb × {lift.topReps}
            </p>
            <!-- The delta is absent, not zero, when the lift is new to this day
                 or when there is no server answer to draw it from. A "0%"
                 there would claim the weight held when nothing was compared. -->
            {#if lift.weightDeltaPct != null && lift.previousTopWeightLb != null}
              <p
                class="text-xs tabular-nums"
                class:text-primary={lift.weightDeltaPct > 0}
                class:text-muted-foreground={lift.weightDeltaPct <= 0}
              >
                {formatDelta(lift.weightDeltaPct)} from {formatVolume(lift.previousTopWeightLb)}
              </p>
            {:else if lift.previousTopWeightLb == null}
              <p class="text-xs text-muted-foreground">new to this day</p>
            {/if}
          </div>
        </li>
      {/each}
    </ul>
  </Card>
{/if}
