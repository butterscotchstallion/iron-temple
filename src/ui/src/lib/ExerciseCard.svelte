<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import PlateBar from "./PlateBar.svelte";
  import { plateLabel } from "./plates";
  import { warmupSets } from "./warmup";
  import type { SessionSet } from "./api";

  let {
    name,
    sets,
    onCycle,
    onChangeWeight,
  }: {
    name: string;
    sets: SessionSet[];
    onCycle: (set: SessionSet) => void;
    onChangeWeight: (delta: number) => void;
  } = $props();

  const workWeight = $derived(sets[0]?.weightLb ?? 0);
  const targetReps = $derived(sets[0]?.targetReps ?? 0);

  // Warm-up ramp expanded to one entry per set (the empty bar is done twice).
  const warmups = $derived.by(() => {
    const out: { weightLb: number; reps: number }[] = [];
    for (const w of warmupSets(workWeight)) {
      for (let k = 0; k < w.sets; k++) {
        out.push({ weightLb: w.weightLb, reps: w.reps });
      }
    }
    return out;
  });

  // Warm-ups aren't persisted (they're a guide), so their completion is local.
  let warmupDone = $state<boolean[]>([]);
  $effect(() => {
    if (warmupDone.length !== warmups.length) {
      warmupDone = warmups.map((_, i) => warmupDone[i] ?? false);
    }
  });

  // The combined sequence is warm-ups (0..w-1) then work sets (w..).
  const total = $derived(warmups.length + sets.length);
  function isDone(i: number): boolean {
    if (i < warmups.length) return warmupDone[i] ?? false;
    return sets[i - warmups.length]?.completed ?? false;
  }
  // The active step drives the plate guide: the first set not yet done, so the
  // ramp auto-advances warm-ups → work like StrongLifts.
  const active = $derived.by(() => {
    for (let i = 0; i < total; i++) if (!isDone(i)) return i;
    return Math.max(0, total - 1);
  });
  const activeWeight = $derived(
    active < warmups.length ? warmups[active].weightLb : workWeight,
  );
  const activeReps = $derived(
    active < warmups.length ? warmups[active].reps : targetReps,
  );

  function warmClass(i: number): string {
    const ring = active === i ? "ring-2 ring-orange-400 ring-offset-2 ring-offset-card " : "";
    const done = warmupDone[i]
      ? "border-orange-500 bg-orange-500 text-background"
      : "border-orange-500/60 bg-orange-500/10 text-orange-500";
    return ring + done;
  }

  function workClass(set: SessionSet, i: number): string {
    const ring =
      active === warmups.length + i
        ? "ring-2 ring-primary ring-offset-2 ring-offset-card "
        : "";
    if (set.actualReps == null || set.actualReps === 0) {
      return ring + "border-border bg-transparent text-muted-foreground";
    }
    if (set.completed) {
      return ring + "border-primary bg-primary text-primary-foreground";
    }
    return ring + "border-primary bg-primary/20 text-foreground";
  }
</script>

<Card class="p-5">
  <div class="flex items-center justify-between gap-3">
    <h3 class="text-lg font-bold text-card-foreground">{name}</h3>
    <div class="flex items-center gap-3">
      <span class="text-sm tabular-nums text-muted-foreground">
        {targetReps} reps
      </span>
      <div class="flex items-center gap-1.5">
        <Button
          variant="outline"
          size="icon-sm"
          onclick={() => onChangeWeight(-5)}
          aria-label="Decrease weight by 5 lb"
        >
          −
        </Button>
        <span
          class="min-w-16 text-center text-sm font-bold tabular-nums text-card-foreground"
        >
          {workWeight} lb
        </span>
        <Button
          variant="outline"
          size="icon-sm"
          onclick={() => onChangeWeight(5)}
          aria-label="Increase weight by 5 lb"
        >
          +
        </Button>
      </div>
    </div>
  </div>

  <!-- Plate guide for the active step (warm-up rung or work weight). -->
  <div class="mt-4 flex flex-col items-center gap-2">
    <PlateBar weightLb={activeWeight} />
    <p class="text-xs tabular-nums text-muted-foreground">
      {activeWeight} lb × {activeReps} · {plateLabel(activeWeight)}
    </p>
  </div>

  <!-- Warm-up circles (orange) then work-set circles (neon), in sequence. -->
  <div class="mt-4 flex flex-wrap items-center gap-x-5 gap-y-3">
    {#if warmups.length > 0}
      <div class="flex flex-wrap items-center gap-2">
        {#each warmups as w, i (i)}
          <button
            type="button"
            class="flex size-11 cursor-pointer items-center justify-center rounded-full border text-sm font-bold tabular-nums transition {warmClass(
              i,
            )}"
            onclick={() => (warmupDone[i] = !warmupDone[i])}
            aria-label={`Warm-up ${w.weightLb} lb × ${w.reps}`}
            aria-pressed={warmupDone[i]}
          >
            {w.reps}
          </button>
        {/each}
      </div>
    {/if}
    <div class="flex flex-wrap items-center gap-3">
      {#each sets as set, i (set.id)}
        <button
          type="button"
          class="flex size-12 cursor-pointer items-center justify-center rounded-full border text-base font-bold tabular-nums transition {workClass(
            set,
            i,
          )}"
          onclick={() => onCycle(set)}
          aria-label={`Set ${set.setNumber}: ${
            set.actualReps == null ? "not logged" : `${set.actualReps} reps`
          }`}
        >
          {set.actualReps ?? 0}
        </button>
      {/each}
    </div>
  </div>

  <p class="mt-3 text-xs text-muted-foreground">
    {#if warmups.length > 0}Orange = warm-up (tap to check off). {/if}Tap a work
    set to add a rep; it clears after the target.
  </p>
</Card>
