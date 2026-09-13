<script lang="ts">
  import type { SessionRecapEarned } from "./api";
  import { Card } from "$lib/components/ui/card";
  import { Badge } from "$lib/components/ui/badge";
  import ArrowUpRight from "@lucide/svelte/icons/arrow-up-right";
  import { formatVolume } from "./volume";

  // What the session bought.
  //
  // The rest of the recap looks backwards; this is the one part that looks
  // forward, and it is the reason a deload stops being a surprise. The engine
  // already explains itself — status, and how many consecutive failures sit
  // behind it — so the weight going down can read as a decision taken on
  // Tuesday rather than as a number that changed overnight.
  //
  // Absent on any but the most recent session of the day: the prescription is
  // computed from history as it stands now, so on an older recap it would
  // describe a workout that has since been superseded. The server withholds it
  // rather than qualifying it, and this component simply never renders.

  let { earned }: { earned: SessionRecapEarned } = $props();

  /** What the engine decided, in the lifter's words rather than the enum's. */
  function verdict(status: string, failures: number, threshold: number): string | null {
    switch (status) {
      case "advance":
        return "up";
      case "deload":
        return "deload";
      case "hold":
        // The count is the useful part: "held" says nothing about how close the
        // next deload is, and a lifter who can see it coming can do something.
        return failures > 0 ? `held · ${failures}/${threshold} to deload` : "held";
      case "layoff":
        return "eased back";
      case "progressing":
        return "reps up";
      // "start" is a lift with no history, and "fixed" is assistance carrying
      // its weight forward. Neither is news.
      default:
        return null;
    }
  }
</script>

<Card class="p-4" data-testid="recap-earned">
  <h3 class="flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-muted-foreground">
    <ArrowUpRight class="size-4 shrink-0" aria-hidden="true" />
    Next {earned.programDayName}
  </h3>
  <ul class="mt-2 divide-y divide-border">
    {#each earned.exercises as exercise (exercise.exerciseId)}
      {@const note = verdict(
        exercise.progression.status,
        exercise.progression.failureCount,
        exercise.progression.failuresBeforeDeload,
      )}
      <li class="flex items-baseline gap-3 py-2">
        <span class="min-w-0 flex-1 truncate text-foreground">{exercise.exerciseName}</span>
        {#if note}
          <Badge variant={exercise.progression.status === "deload" ? "destructive" : "secondary"}>
            {note}
          </Badge>
        {/if}
        <span class="shrink-0 tabular-nums text-foreground">
          {exercise.sets}×{exercise.reps}
          {#if exercise.repMax}<span class="text-muted-foreground">-{exercise.repMax}</span>{/if}
          @ {formatVolume(exercise.weightLb)} lb
        </span>
      </li>
    {/each}
  </ul>
</Card>
