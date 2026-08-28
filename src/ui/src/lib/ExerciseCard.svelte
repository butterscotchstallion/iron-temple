<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import Minus from "@lucide/svelte/icons/minus";
  import Plus from "@lucide/svelte/icons/plus";
  import PlateBar from "./PlateBar.svelte";
  import { plateLabel } from "./plates";
  import { barWeightLb, plateInventory } from "./gym.svelte";
  import { warmupSets } from "./warmup";
  import { formatTime } from "./time";
  import type { SessionSet } from "./api";

  let {
    name,
    sets,
    onCycle,
    onChangeWeight,
    onAddSet,
    onRemoveSet,
    readonly = false,
  }: {
    name: string;
    sets: SessionSet[];
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

  // Warm-up ramp expanded to one entry per set (the empty bar is done twice).
  // Built against this lifter's bar and rack: the ramp starts at whatever their
  // bar weighs and each rung rounds to a weight that rack can build, so a
  // prescription below the bar yields no warm-ups rather than impossible ones.
  // A ramp is its own warm-up — that is what the first three rungs of a Madcow
  // day are — so bolting a second one in front of it would have the lifter warm
  // up to warm up.
  const warmups = $derived.by(() => {
    const out: { weightLb: number; reps: number }[] = [];
    if (ramping) return out;
    for (const w of warmupSets(workWeight, barWeightLb(), plateInventory())) {
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
    active < warmups.length
      ? warmups[active].weightLb
      : (sets[active - warmups.length]?.weightLb ?? workWeight),
  );
  const activeReps = $derived(
    active < warmups.length
      ? warmups[active].reps
      : (sets[active - warmups.length]?.targetReps ?? targetReps),
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
      {activeWeight} lb × {activeReps} · {plateLabel(
        activeWeight,
        barWeightLb(),
        plateInventory(),
      )}
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
