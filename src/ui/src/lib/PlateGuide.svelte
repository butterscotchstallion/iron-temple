<script lang="ts">
  import PlateBar from "./PlateBar.svelte";
  import { plateLabel } from "./plates";
  import type { WarmupSet } from "./warmup";

  let {
    warmups,
    workWeightLb,
    workReps,
    workSets,
  }: {
    warmups: WarmupSet[];
    workWeightLb: number;
    workReps: number;
    workSets: number;
  } = $props();

  type Step = { weightLb: number; reps: number; sets: number; work: boolean };

  // The full ramp: warm-up rungs, then the work set as the final step.
  const steps = $derived<Step[]>([
    ...warmups.map((w) => ({ ...w, work: false })),
    { weightLb: workWeightLb, reps: workReps, sets: workSets, work: true },
  ]);

  // Which step's plates to show; null means "the work set" (the default).
  let selected = $state<number | null>(null);
  const active = $derived(
    selected != null && selected < steps.length ? selected : steps.length - 1,
  );
  const step = $derived(steps[active]);
</script>

<div class="flex flex-col items-center gap-3">
  {#if steps.length > 1}
    <div class="flex flex-wrap justify-center gap-1.5">
      {#each steps as s, i (i)}
        <button
          type="button"
          class="cursor-pointer rounded-full border px-3 py-1.5 text-xs font-bold tabular-nums transition {i ===
          active
            ? 'border-primary bg-primary text-primary-foreground'
            : s.work
              ? 'border-primary/50 text-primary hover:bg-primary/10'
              : 'border-border text-muted-foreground hover:border-primary/50'}"
          onclick={() => (selected = i)}
          aria-pressed={i === active}
        >
          {s.work ? "Work" : s.weightLb}
        </button>
      {/each}
    </div>
  {/if}

  <PlateBar weightLb={step.weightLb} />

  <p class="text-xs tabular-nums text-muted-foreground">
    {#if step.sets > 1}{step.sets} ×
    {/if}{step.weightLb} lb × {step.reps} · {plateLabel(step.weightLb)}
  </p>
</div>
