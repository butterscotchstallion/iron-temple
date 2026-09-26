<script lang="ts">
  import { type Exercise } from "./api";
  import { displayLb, displayStepLb, storedLb, weightUnitLabel } from "./library";
  import { DEFAULT_BAR_STEP_LB } from "./plates";
  import { gymSteps } from "./gym.svelte";
  import { exerciseEmoji } from "./exerciseIcon";
  import { formatVolume } from "./volume";
  import { Button } from "$lib/components/ui/button";
  import ExercisePicker from "./ExercisePicker.svelte";
  import Plus from "@lucide/svelte/icons/plus";

  // Prescribing an accessory: ExercisePicker chooses the movement, and this
  // adds the numbers that make it a plan.
  //
  // The picker half used to live here. It moved out when the program editor
  // needed the same list — and only the list, because a program's own
  // prescription asks where a lift STARTS (a number read only until there is
  // history) while this asks what it carries FORWARD, and the rep range below
  // has no counterpart there at all. Two step-twos with no field in common is
  // an extraction rather than a flag.
  let {
    exclude = [],
    onAdd,
    onCancel,
    confirmLabel = "Add to this day",
    footnote,
  }: {
    // Exercise ids already on this day — one entry per lift, so offering them
    // again would only earn a 409.
    exclude?: number[];
    onAdd: (
      choice: {
        exerciseId: number;
        sets: number;
        reps: number;
        weightLb: number;
        repMin?: number;
        repMax?: number;
      },
      exercise: Exercise,
    ) => Promise<boolean>;
    onCancel: () => void;
    /**
     * What the confirm button says. Defaults to the program page's wording; the
     * session screen adds the lift to the workout in front of the lifter as
     * well as to the day, and a button naming only the day would be describing
     * the half they cannot see.
     */
    confirmLabel?: string;
    /** An extra line under the inputs, for whatever else the caller is doing. */
    footnote?: string;
  } = $props();

  let selected = $state<Exercise | null>(null);

  // Three sets of five at bodyweight: the default nearly every accessory starts
  // at, and all three are editable before adding.
  let sets = $state(3);
  let reps = $state(5);
  let weightLb = $state(0);
  let saving = $state(false);

  // Off by default, and this has now defaulted both ways, so it is worth
  // recording why rather than quietly flipping back again.
  //
  // It was ON because of an argument about dumbbells: a pair steps 10 lb, the
  // step cannot be made finer, and so the only gentle lever left was to advance
  // less OFTEN — which is what climbing 8 to 12 inside one weight does. That
  // reasoning was sound and its premise was wrong. The pair never stepped 10 in
  // the lifter's hand; it stepped one bell, and the app was reporting one jump
  // as two by showing the whole load. Now that a dumbbell reads per bell, the
  // linear rule advances a curl 30 → 35 — the next bell in the rack, which is
  // the smallest increase that gym can express and needs no softening.
  //
  // What the default cost in the meantime was legibility. A range prescribes its
  // BOTTOM (see `reps` below), so every accessory read "8 reps" beside a program
  // of fives, forever, with nothing on the workout screen mentioning a range or
  // asking anyone to climb to 12. A mechanism a lifter cannot see is not a
  // gentler pace, it is an unexplained number.
  //
  // So the linear rule is the default and the range is the opt-in, which is the
  // way round that matches what each one is: one is what an accessory does, the
  // other is a deliberate choice about a lift that wants a slower climb.
  let ranged = $state(false);
  let repMin = $state(8);
  let repMax = $state(12);

  // What the chosen movement's weight moves in, in THIS lifter's gym, in the
  // units the box below is in: their lightest plate doubled on a bar, the next
  // bell up on a rack. Drives both the copy below and the number input's step,
  // so the arrows offer weights the rack can actually make.
  const stepLb = $derived(
    selected ? displayStepLb(selected.equipment, gymSteps()) : DEFAULT_BAR_STEP_LB,
  );
  // Both halves of the border, bound to the selected movement. `shown` fills the
  // weight box from the library's stored top set; `confirm` sends `storedLb`
  // back, because the API stores the whole load however the box says it.
  const shown = (lb: number) => (selected ? displayLb(lb, selected.equipment) : lb);
  const unit = $derived(selected ? weightUnitLabel(selected.equipment) : "lb");

  /**
   * Pick a movement, and start its weight where the lifter left it.
   *
   * Zero is the right default for a first-ever accessory and the wrong one for
   * a lift already trained — nobody adds dips meaning to do them at bodyweight
   * when they have been adding 25 lb for a month, and retyping it at the rack
   * is the kind of small friction that stops the lift being logged at all.
   *
   * topSet is the HEAVIEST set ever, not the last one. It is what the library
   * carries, and the caption below says so rather than calling it "last time" —
   * the server's own carry-forward uses the last performance, and the two are
   * different numbers after a deload. A starting point to adjust, labelled
   * honestly, beats an empty box.
   */
  function choose(exercise: Exercise) {
    selected = exercise;
    // Into the box's units — topSet is stored as the whole load.
    weightLb = displayLb(exercise.topSet?.weightLb ?? 0, exercise.equipment);
  }

  async function confirm() {
    if (!selected || saving) return;
    saving = true;
    const ok = await onAdd(
      {
        exerciseId: selected.id,
        sets,
        // With a range the bottom is the rep target: a set is complete at the
        // bottom and the weight moves at the top.
        reps: ranged ? repMin : reps,
        // Back to the whole load, which is what the API and the engine store.
        weightLb: storedLb(weightLb, selected.equipment),
        ...(ranged ? { repMin, repMax } : {}),
      },
      // The movement itself, beside the numbers rather than folded into them:
      // the first argument is a request body, and a caller that adds this lift
      // to a live workout needs the name and the rest length to draw its sets
      // before the server has answered.
      selected,
    );
    saving = false;
    // On failure the parent shows the banner and the panel stays open with the
    // selection intact, so the numbers needn't be typed twice.
    if (ok) selected = null;
  }

</script>

<div class="mt-3 flex flex-col gap-3 rounded-md border border-border/60 p-3">
  {#if selected}
    <!-- Step two: prescribe it. -->
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
        <input
          type="number"
          min="1"
          max="20"
          bind:value={sets}
          class="rounded-md border border-input bg-transparent px-2 py-1.5 text-sm tabular-nums text-foreground outline-none transition focus:border-primary"
        />
      </label>
      {#if !ranged}
        <label class="flex flex-1 flex-col gap-1 text-xs text-muted-foreground">
          Reps
          <input
            type="number"
            min="1"
            max="100"
            bind:value={reps}
            class="rounded-md border border-input bg-transparent px-2 py-1.5 text-sm tabular-nums text-foreground outline-none transition focus:border-primary"
          />
        </label>
      {:else}
        <label class="flex flex-1 flex-col gap-1 text-xs text-muted-foreground">
          Reps from
          <input
            type="number"
            min="1"
            max="100"
            bind:value={repMin}
            class="rounded-md border border-input bg-transparent px-2 py-1.5 text-sm tabular-nums text-foreground outline-none transition focus:border-primary"
          />
        </label>
        <label class="flex flex-1 flex-col gap-1 text-xs text-muted-foreground">
          up to
          <input
            type="number"
            min="1"
            max="100"
            bind:value={repMax}
            class="rounded-md border border-input bg-transparent px-2 py-1.5 text-sm tabular-nums text-foreground outline-none transition focus:border-primary"
          />
        </label>
      {/if}
      <label class="flex flex-1 flex-col gap-1 text-xs text-muted-foreground">
        Weight ({unit})
        <input
          type="number"
          min="0"
          step={stepLb}
          bind:value={weightLb}
          class="rounded-md border border-input bg-transparent px-2 py-1.5 text-sm tabular-nums text-foreground outline-none transition focus:border-primary"
        />
      </label>
    </div>
    <label class="flex items-center gap-2 text-xs text-muted-foreground">
      <input type="checkbox" bind:checked={ranged} class="size-4 accent-primary" />
      Use a rep range
    </label>
    {#if selected?.topSet}
      <!-- Named for what it is. topSet is the heaviest set ever, and the
           carry-forward the server applies from the next session on uses the
           LAST one — after a deload those disagree, and calling this "last
           time" would quietly be a lie. -->
      <p class="text-xs tabular-nums text-muted-foreground">
        Your heaviest so far: {formatVolume(shown(selected.topSet.weightLb))}
        {unit}
      </p>
    {/if}
    <p class="text-xs text-muted-foreground">
      Leave the weight at 0 for bodyweight work.
      {#if ranged}
        With a range, hit the top on every set and the weight goes up {stepLb}
        {unit} next time, with the reps back at the bottom. It never deloads — good
        for light work where {stepLb} a session is too big a jump.
      {:else}
        Hit your reps on every set and it goes up {stepLb}
        {unit} next time, the same as the program's own lifts. Miss and it stays;
        miss three times and it drops back. Bodyweight work stays bodyweight.
      {/if}
    </p>
    {#if footnote}
      <p class="text-xs text-muted-foreground">{footnote}</p>
    {/if}
    <div class="flex gap-2">
      <Button size="sm" onclick={confirm} disabled={saving}>
        <Plus />
        {saving ? "Adding…" : confirmLabel}
      </Button>
      <Button size="sm" variant="ghost" onclick={onCancel}>Cancel</Button>
    </div>
  {:else}
    <ExercisePicker
      {exclude}
      onPick={choose}
      {onCancel}
    />
  {/if}
</div>
