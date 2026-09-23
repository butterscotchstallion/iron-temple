<script lang="ts">
  import { type Exercise } from "./api";
  import { equipmentStepLb } from "./library";
  import { DEFAULT_BAR_STEP_LB } from "./plates";
  import { gymSteps } from "./gym.svelte";
  import { exerciseEmoji } from "./exerciseIcon";
  import { formatVolume } from "./volume";
  import { Button } from "$lib/components/ui/button";
  import ExercisePicker from "./ExercisePicker.svelte";
  import Plus from "@lucide/svelte/icons/plus";

  // Adding a lift to a program's own prescription.
  //
  // AssistancePicker's sibling, and the differences are the reason they are two
  // components rather than one with a flag:
  //
  //  - The weight here is where the lift STARTS, consulted only while it has no
  //    history. Assistance's is what the lift carries forward, applied every
  //    session until it is next performed.
  //  - There is no rep range. Double progression is an assistance rule; a
  //    program's own lifts run the linear engine, which is what makes "hit your
  //    reps, add weight" the thing the app is for.
  //  - Adding here changes the program, so it reaches everybody training it.
  //    Adding assistance changes nothing anybody else can see.
  let {
    exclude = [],
    onAdd,
    onCancel,
  }: {
    exclude?: number[];
    onAdd: (choice: {
      exerciseId: number;
      sets: number;
      reps: number;
      startingWeightLb: number;
    }) => Promise<boolean>;
    onCancel: () => void;
  } = $props();

  let selected = $state<Exercise | null>(null);
  // Five by five: what every program this app ships starts from, and all three
  // editable before adding.
  let sets = $state(5);
  let reps = $state(5);
  let startingWeightLb = $state(0);
  let saving = $state(false);

  // What this movement's weight moves in, in THIS lifter's gym — twice their
  // lightest plate on a bar, twice their rack's step on a pair of bells. Drives
  // the number input's step, so the arrows offer weights the rack can make.
  const stepLb = $derived(
    selected ? equipmentStepLb(selected.equipment, gymSteps()) : DEFAULT_BAR_STEP_LB,
  );

  const inputClass =
    "rounded-md border border-input bg-transparent px-2 py-1.5 text-sm tabular-nums text-foreground outline-none transition focus:border-primary";

  /**
   * Pick a movement, and suggest where it starts from what they have lifted.
   *
   * The starting weight only ever applies while a lift has NO history, so for a
   * lift they already train this box changes nothing whatever it says. Filling
   * it in anyway is still the better default: it is right for the case it
   * matters in — a lift they are about to start — and for the case it does not,
   * it at least shows a number in the right neighbourhood rather than a zero
   * that reads like a claim.
   */
  function choose(exercise: Exercise) {
    selected = exercise;
    startingWeightLb = exercise.topSet?.weightLb ?? 0;
  }

  async function confirm() {
    if (!selected || saving) return;
    saving = true;
    const ok = await onAdd({
      exerciseId: selected.id,
      sets,
      reps,
      startingWeightLb,
    });
    saving = false;
    // On failure the parent shows the banner and this stays open with the
    // selection intact, so the numbers needn't be typed twice.
    if (ok) selected = null;
  }
</script>

<div class="mt-3 flex flex-col gap-3 rounded-md border border-border/60 p-3">
  {#if selected}
    <div class="flex items-center gap-2">
      <span class="text-xl" aria-hidden="true">{exerciseEmoji(selected.name)}</span>
      <span class="flex-1 font-semibold text-card-foreground">{selected.name}</span>
      <button
        type="button"
        class="text-xs font-semibold text-muted-foreground underline underline-offset-2 transition hover:text-foreground"
        onclick={() => (selected = null)}
      >
        Change
      </button>
    </div>

    <div class="flex flex-wrap gap-3">
      <label class="flex flex-1 flex-col gap-1 text-xs text-muted-foreground">
        Sets
        <input type="number" min="1" max="20" bind:value={sets} class={inputClass} />
      </label>
      <label class="flex flex-1 flex-col gap-1 text-xs text-muted-foreground">
        Reps
        <input type="number" min="1" max="100" bind:value={reps} class={inputClass} />
      </label>
      <label class="flex flex-1 flex-col gap-1 text-xs text-muted-foreground">
        Starts at (lb)
        <input
          type="number"
          min="0"
          max="2000"
          step={stepLb}
          bind:value={startingWeightLb}
          class={inputClass}
        />
      </label>
    </div>

    {#if selected?.topSet}
      <!-- Named for what it is. topSet is the heaviest set ever; the engine
           advances from the LAST one, and after a deload those disagree. -->
      <p class="text-xs tabular-nums text-muted-foreground">
        Your heaviest so far: {formatVolume(selected.topSet.weightLb)} lb
      </p>
    {/if}

    <p class="text-xs text-muted-foreground">
      The starting weight only applies until you've lifted this — after that the
      weights come from what you actually did, so a lift you already train picks
      up where you left it. Leave it at 0 for bodyweight work.
    </p>

    <div class="flex gap-2">
      <Button size="sm" onclick={confirm} disabled={saving}>
        <Plus />
        {saving ? "Adding…" : "Add to this day"}
      </Button>
      <Button size="sm" variant="ghost" onclick={onCancel}>Cancel</Button>
    </div>
  {:else}
    <ExercisePicker {exclude} onPick={choose} {onCancel} />
  {/if}
</div>
