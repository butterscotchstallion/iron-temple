<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import Minus from "@lucide/svelte/icons/minus";
  import Plus from "@lucide/svelte/icons/plus";
  import PlateBar from "./PlateBar.svelte";
  import { plateLabel } from "./plates";
  import { equipmentStepLb } from "./library";
  import { barWeightLb, gymSteps, plateInventory } from "./gym.svelte";
  import { warmupSets } from "./warmup";
  import { formatTime } from "./time";
  import type { SessionSet } from "./api";

  let {
    name,
    sets,
    equipment = "barbell",
    onCycle,
    onChangeWeight,
    onAddSet,
    onRemoveSet,
    readonly = false,
  }: {
    name: string;
    sets: SessionSet[];
    /**
     * The movement's equipment. Everything this card says about loading a
     * weight depends on it: a barbell gets an empty-bar opener, a plate diagram
     * and 5 lb steps, and a pair of dumbbells gets none of those, because none
     * of them exist for a pair of dumbbells. Defaulted rather than required so a
     * card still renders without one, the way `readonly` is.
     */
    equipment?: string;
    onCycle: (set: SessionSet) => void;
    onChangeWeight: (delta: number) => void;
    /** Append one more set of this lift. Omitted where sets are fixed. */
    onAddSet?: () => void;
    /** Drop a set that wasn't performed. Omitted where sets are fixed. */
    onRemoveSet?: (set: SessionSet) => void;
    // An over session is a record, not a worksheet: sets and weights lock.
    readonly?: boolean;
  } = $props();

  // The set the lifter has asked to remove, held until they confirm. Only reps
  // that were actually logged are worth a confirmation — dropping an untouched
  // set is the same gesture as never having had it, and a dialog there is a
  // dialog in the way of somebody between sets.
  let pendingRemoval = $state<SessionSet | null>(null);

  function requestRemove(set: SessionSet) {
    if (readonly) return;
    if (set.actualReps == null || set.actualReps === 0) {
      onRemoveSet?.(set);
      return;
    }
    pendingRemoval = set;
  }

  function confirmRemove() {
    if (pendingRemoval) onRemoveSet?.(pendingRemoval);
    pendingRemoval = null;
  }

  // Only a barbell has a bar to load, so only a barbell gets the diagram, the
  // per-side plate line and the empty-bar opener in front of its work sets.
  const barbell = $derived(equipment === "barbell");
  // The smallest change this equipment admits IN THIS GYM, which is what the
  // stepper should move by: twice the lightest plate owned on a bar, twice the
  // rack's step on a pair of dumbbells. Shared with the API's progression.Ladder
  // through `equipmentStepLb`, so the button and the engine agree about what the
  // next weight up even is — ±5 on a lift whose rack steps 5 lb a bell asks for
  // a 35 lb pair nobody owns.
  const stepLb = $derived(equipmentStepLb(equipment, gymSteps()));

  // The last set is the one a "remove a set" control should target: sets are
  // numbered in order and the tail is what an extra one was appended to.
  const lastSet = $derived(sets[sets.length - 1]);

  // A ramping lift gives every set its own weight and reps — Madcow climbs
  // 50/62.5/75/87.5/100% of a top set, and its intensity day finishes with a
  // triple above that and a backoff below it. A uniform block is the common case
  // and keeps the compact display it has always had.
  const ramping = $derived(
    sets.some(
      (s) => s.weightLb !== sets[0]?.weightLb || s.targetReps !== sets[0]?.targetReps,
    ),
  );
  // The top set is the heaviest, which is the number a ramping lift is "about":
  // it is what the percentages are of and what moves week to week. For a uniform
  // block it is simply the weight.
  const workWeight = $derived(
    ramping
      ? Math.max(...sets.map((s) => s.weightLb))
      : (sets[0]?.weightLb ?? 0),
  );
  const targetReps = $derived(sets[0]?.targetReps ?? 0);
  // The rest this lift asks for, shown alongside the rep target because it is
  // half of the prescription and the countdown that enforces it lives in a
  // corner of the screen with no name on it.
  const restSeconds = $derived(sets[0]?.restSeconds ?? 0);

  // Warm-up ramp expanded to one entry per set (on a barbell the empty bar is
  // done twice). Built against this lifter's bar and rack and this lift's
  // equipment: a barbell ramp starts at whatever their bar weighs and each rung
  // rounds to a weight that rack can build, so a prescription below the bar
  // yields no warm-ups rather than impossible ones — and a lift with no bar
  // ramps from nothing in the steps its own equipment admits.
  // A ramp is its own warm-up — that is what the first three rungs of a Madcow
  // day are — so bolting a second one in front of it would have the lifter warm
  // up to warm up.
  //
  // Capped at the number of work sets: the warm-up never outnumbers the lift it
  // is warming up for. This card is where the cap belongs because this is what
  // knows how many sets today prescribes — a 5x5 keeps the whole ramp, a 2x5
  // gets the two rungs closest to the work weight.
  //
  // The cap follows the prescription only until the warm-up is done, then it
  // stops following it. Sets get added at the rack — an extra one, an AMRAP —
  // and a lifter who is already through their ramp has warmed up for the lift
  // they are in the middle of. Growing the ramp under them would put a fresh
  // cyan circle on the card and ask them to go back and do it, and because the
  // trim drops rungs from the light end, the new one lands at the front and
  // shifts every rep they logged along with it. So `cappedAt` is null until the
  // ramp is finished and the set count at that moment after — pinned rather
  // than tracked as a high-water mark, so undoing the extra set doesn't bring
  // the rung back either. The warm-up happened; nothing after it changes that.
  let cappedAt = $state<number | null>(null);
  const rampCap = $derived(cappedAt ?? sets.length);
  const warmups = $derived.by(() => {
    const out: { weightLb: number; reps: number }[] = [];
    if (ramping) return out;
    for (const w of warmupSets(workWeight, {
      equipment,
      bar: barWeightLb(),
      plates: plateInventory(),
      steps: gymSteps(),
      maxSets: rampCap,
    })) {
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

  // Every rung tapped through to its target. A lift with no ramp is never
  // "complete" — there is nothing to have finished, and a bar-weight lift that
  // gets loaded up mid-session should still get the warm-up it now needs.
  const warmupComplete = $derived(
    warmups.length > 0 && warmups.every((_, i) => warmDone(i)),
  );
  $effect(() => {
    if (warmupComplete && cappedAt === null) cappedAt = sets.length;
  });

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
    active < warmups.length
      ? warmups[active].weightLb
      : (sets[active - warmups.length]?.weightLb ?? workWeight),
  );
  const activeReps = $derived(
    active < warmups.length
      ? warmups[active].reps
      : (sets[active - warmups.length]?.targetReps ?? targetReps),
  );

  // How the active weight is actually carried, said in the terms the equipment
  // uses. A barbell gets the plates for one side. A dumbbell gets the weight of
  // one bell, because every weight in this app is the whole load and the number
  // stamped on the thing in the lifter's hand is half of it. A machine's stack
  // and a cable's pin are their own label already, so they get nothing rather
  // than a sentence restating the weight above them.
  const loadNote = $derived.by(() => {
    if (barbell) return plateLabel(activeWeight, barWeightLb(), plateInventory());
    if (equipment === "dumbbell") return `${activeWeight / 2} lb per hand`;
    return "";
  });

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
        <!-- A ramp has no single rep target, so it says what it is instead:
             how many sets are coming, up to the top set beside it. -->
        {#if ramping}
          {sets.length} sets, ramping
        {:else}
          {targetReps} reps
        {/if}
        {#if restSeconds > 0}
          · {formatTime(restSeconds)} rest
        {/if}
      </span>
      <div class="flex items-center gap-1.5">
        <Button
          variant="outline"
          size="icon-sm"
          onclick={() => stepWeight(-stepLb)}
          disabled={readonly}
          aria-label="Decrease weight by {stepLb} lb"
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
          onclick={() => stepWeight(stepLb)}
          disabled={readonly}
          aria-label="Increase weight by {stepLb} lb"
        >
          <Plus />
        </Button>
      </div>
    </div>
  </div>

  <!-- Loading guide for the active step (warm-up rung or work weight). The bar
       diagram is drawn only where there is a bar; off the barbell the weight and
       reps carry the step on their own, plus whatever `loadNote` has to add. -->
  <div class="mt-4 flex flex-col items-center gap-2">
    {#if barbell}
      <PlateBar weightLb={activeWeight} />
    {/if}
    <p class="text-xs tabular-nums text-muted-foreground">
      {activeWeight} lb × {activeReps}{loadNote ? ` · ${loadNote}` : ""}
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
          title={ramping ? `${set.weightLb} lb x ${set.targetReps}` : undefined}
          aria-label={ramping
            ? `Set ${set.setNumber}, ${set.weightLb} lb for ${set.targetReps}: ${
                set.actualReps == null ? "not logged" : `${set.actualReps} reps`
              }`
            : `Set ${set.setNumber}: ${
                set.actualReps == null ? "not logged" : `${set.actualReps} reps`
              }`}
        >
          {set.actualReps ?? 0}
        </button>
      {/each}

      <!-- Add and drop a set. The prescription is a plan, not a cage: an extra
           set, an AMRAP or a set skipped all happen, and until these existed the
           closest a lifter could get was a ghost row logged at zero reps. -->
      {#if !readonly && (onAddSet || onRemoveSet)}
        <div class="flex items-center gap-1.5">
          {#if onRemoveSet && lastSet}
            <Button
              variant="ghost"
              size="icon-sm"
              onclick={() => requestRemove(lastSet)}
              aria-label={`Remove set ${lastSet.setNumber} of ${name}`}
            >
              <Minus />
            </Button>
          {/if}
          {#if onAddSet}
            <Button
              variant="ghost"
              size="icon-sm"
              onclick={onAddSet}
              aria-label={`Add a set of ${name}`}
            >
              <Plus />
            </Button>
          {/if}
        </div>
      {/if}
    </div>
  </div>

  <AlertDialog.Root
    open={pendingRemoval !== null}
    onOpenChange={(open) => {
      if (!open) pendingRemoval = null;
    }}
  >
    <AlertDialog.Content>
      <AlertDialog.Header>
        <AlertDialog.Title>Remove this set?</AlertDialog.Title>
        <AlertDialog.Description>
          Set {pendingRemoval?.setNumber} of {name} has {pendingRemoval?.actualReps}
          {pendingRemoval?.actualReps === 1 ? "rep" : "reps"} logged against it.
          Removing it throws that away.
        </AlertDialog.Description>
      </AlertDialog.Header>
      <AlertDialog.Footer>
        <AlertDialog.Cancel>Keep it</AlertDialog.Cancel>
        <AlertDialog.Action onclick={confirmRemove}>Remove</AlertDialog.Action>
      </AlertDialog.Footer>
    </AlertDialog.Content>
  </AlertDialog.Root>

  {#if ramping}
    <!-- The ramp written out. Each circle above is one of these, but a lifter
         setting up the bar wants to see the whole climb at once. -->
    <ol class="mt-3 flex flex-wrap gap-x-3 gap-y-1 text-xs tabular-nums text-muted-foreground">
      {#each sets as set (set.id)}
        <li class={active === warmups.length + sets.indexOf(set) ? "text-primary" : ""}>
          {set.weightLb}×{set.targetReps}
        </li>
      {/each}
    </ol>
  {/if}

  <p class="mt-3 text-xs text-muted-foreground">
    {#if readonly}
      This workout is finished — sets are locked.
    {:else}
      {#if warmups.length > 0}Cyan sets are warm-ups. {/if}{#if ramping}The ramp
        is the warm-up — work up through it. {/if}Tap a set to add a rep; it
      clears after the target.
    {/if}
  </p>
</Card>
