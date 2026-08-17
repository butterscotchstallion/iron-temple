<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import Minus from "@lucide/svelte/icons/minus";
  import Plus from "@lucide/svelte/icons/plus";
  import PlateBar from "./PlateBar.svelte";
  import { plateLabel } from "./plates";
  import { warmupSets } from "./warmup";
  import type { SessionSet } from "./api";

  let {
    name,
    sets,
    onCycle,
    onChangeWeight,
    readonly = false,
  }: {
    name: string;
    sets: SessionSet[];
    onCycle: (set: SessionSet) => void;
    onChangeWeight: (delta: number) => void;
    // An over session is a record, not a worksheet: sets and weights lock.
    readonly?: boolean;
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

  // Warm-ups aren't persisted (they're a guide), so reps are tracked locally.
  // They count up from 0 to the target then clear, just like work sets.
  let warmupReps = $state<(number | null)[]>([]);
  $effect(() => {
    if (warmupReps.length !== warmups.length) {
      warmupReps = warmups.map((_, i) => warmupReps[i] ?? null);
    }
  });

  function warmDone(i: number): boolean {
    const r = warmupReps[i];
    return r != null && r >= warmups[i].reps;
  }

  function cycleWarmup(i: number) {
    if (readonly) return;
    const cur = warmupReps[i];
    const target = warmups[i].reps;
    warmupReps[i] = cur == null ? 1 : cur >= target ? null : cur + 1;
  }

  // The combined sequence is warm-ups (0..w-1) then work sets (w..).
  const total = $derived(warmups.length + sets.length);
  function isDone(i: number): boolean {
    if (i < warmups.length) return warmDone(i);
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
    const ring =
      active === i ? "ring-2 ring-cyan ring-offset-2 ring-offset-card " : "";
    const reps = warmupReps[i];
    if (reps == null || reps === 0) {
      return ring + "border-cyan/40 bg-transparent text-cyan/70";
    }
    if (reps >= warmups[i].reps) {
      return ring + "border-cyan bg-cyan text-background"; // hit target
    }
    return ring + "border-cyan bg-cyan/20 text-foreground"; // in progress
  }

  // The disabled attribute already blocks these in a browser; the explicit
  // guards keep the component correct for any click that arrives anyway.
  function cycleSet(set: SessionSet) {
    if (readonly) return;
    onCycle(set);
  }

  function stepWeight(delta: number) {
    if (readonly) return;
    onChangeWeight(delta);
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
          onclick={() => stepWeight(-5)}
          disabled={readonly}
          aria-label="Decrease weight by 5 lb"
        >
          <Minus />
        </Button>
        <span
          class="min-w-16 text-center text-sm font-bold tabular-nums text-card-foreground"
        >
          {workWeight} lb
        </span>
        <Button
          variant="outline"
          size="icon-sm"
          onclick={() => stepWeight(5)}
          disabled={readonly}
          aria-label="Increase weight by 5 lb"
        >
          <Plus />
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

  <!-- Warm-up circles (cyan) then work-set circles (neon), in sequence. -->
  <div class="mt-4 flex flex-wrap items-center gap-x-5 gap-y-3">
    {#if warmups.length > 0}
      <div class="flex flex-wrap items-center gap-2">
        {#each warmups as w, i (i)}
          <button
            type="button"
            class="flex size-11 items-center justify-center rounded-full border text-sm font-bold tabular-nums transition {readonly
              ? 'cursor-default'
              : 'cursor-pointer'} {warmClass(i)}"
            onclick={() => cycleWarmup(i)}
            disabled={readonly}
            aria-label={`Warm-up ${w.weightLb} lb × ${w.reps}: ${
              warmupReps[i] ?? 0
            } reps`}
          >
            {warmupReps[i] ?? 0}
          </button>
        {/each}
      </div>
    {/if}
    <div class="flex flex-wrap items-center gap-3">
      {#each sets as set, i (set.id)}
        <button
          type="button"
          class="flex size-12 items-center justify-center rounded-full border text-base font-bold tabular-nums transition {readonly
            ? 'cursor-default'
            : 'cursor-pointer'} {workClass(set, i)}"
          onclick={() => cycleSet(set)}
          disabled={readonly}
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
    {#if readonly}
      This workout is finished — sets are locked.
    {:else}
      {#if warmups.length > 0}Cyan sets are warm-ups. {/if}Tap a set to add a
      rep; it clears after the target.
    {/if}
  </p>
</Card>
