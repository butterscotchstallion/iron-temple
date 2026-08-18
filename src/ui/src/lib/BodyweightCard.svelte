<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import Scale from "@lucide/svelte/icons/scale";
  import { formatLongDate } from "./date";
  import type { WeighIn } from "./api";

  let {
    bodyweightLb = null,
    lastWeighIn = null,
    readonly = false,
    onSave,
  }: {
    // This session's own weigh-in. Null means the lifter didn't record one.
    bodyweightLb?: number | null;
    // Their most recent weigh-in from another session, which is what the box
    // opens pre-filled with. Null until they've recorded one anywhere.
    lastWeighIn?: WeighIn | null;
    // An over session is a record, not a worksheet — same lock as the sets.
    readonly?: boolean;
    // Null clears the weigh-in. Awaited only so the box can disable itself
    // while the write is in flight.
    onSave: (weightLb: number | null) => Promise<void> | void;
  } = $props();

  // The bounds the box accepts. Wider than any lifter on either side, because
  // their job is to catch a slipped decimal point rather than to have an
  // opinion about bodies. The API allows more still (up to 2000), so a value
  // this passes can never be one the server rejects.
  const MIN_LB = 50;
  const MAX_LB = 1000;

  let saving = $state(false);
  // Set when the typed value is out of range, cleared on the next good entry.
  // Not an error from the server — this one never leaves the browser.
  let rangeError = $state(false);

  // What the box shows: this session's weigh-in if there is one, otherwise the
  // carried value. Read straight from the props with no local draft, so a saved
  // value can't drift from what the server holds. The DOM keeps whatever is
  // being typed until it's committed — Svelte only rewrites the input when this
  // expression changes, which it doesn't mid-edit.
  //
  // A locked session shows only what it actually recorded. Pre-filling a box
  // nobody can edit would read as a measurement rather than a suggestion.
  const shown = $derived(
    bodyweightLb ?? (readonly ? null : (lastWeighIn?.weightLb ?? null)),
  );

  // Whether the number on screen is this session's record or last time's.
  const carried = $derived(bodyweightLb == null && shown != null);

  const caption = $derived.by(() => {
    if (rangeError) return `Enter a weight between ${MIN_LB} and ${MAX_LB} lb`;
    if (bodyweightLb != null) return "Logged for this session";
    if (readonly) return "No weigh-in";
    if (carried && lastWeighIn) {
      return `Carried from ${formatLongDate(lastWeighIn.performedOn)} · edit to log today`;
    }
    return "First weigh-in — today's number starts the series";
  });

  // Committed on change (blur or Enter), not on every keystroke: a half-typed
  // "18" on the way to "184" is not a weigh-in worth a round trip.
  async function commit(event: Event) {
    if (readonly || saving) return;
    const input = event.currentTarget as HTMLInputElement;

    // An emptied box erases the weigh-in — but only if there was one. Emptying
    // a merely pre-filled box is a lifter declining to weigh in, which is
    // already what an untouched session records.
    if (input.value.trim() === "") {
      rangeError = false;
      if (bodyweightLb == null) return;
      await save(null);
      return;
    }

    const next = input.valueAsNumber;
    if (!Number.isFinite(next) || next < MIN_LB || next > MAX_LB) {
      rangeError = true;
      return;
    }
    rangeError = false;
    // Re-committing the same number (a blur with no edit) is not a change.
    if (next === bodyweightLb) return;
    await save(next);
  }

  async function save(weightLb: number | null) {
    saving = true;
    try {
      await onSave(weightLb);
    } finally {
      saving = false;
    }
  }
</script>

<Card class="flex flex-wrap items-center gap-x-4 gap-y-2 p-5">
  <label
    class="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground"
    for="bodyweight"
  >
    <Scale class="size-4" aria-hidden="true" />
    Bodyweight
  </label>
  <div class="flex items-center gap-2">
    <input
      id="bodyweight"
      type="number"
      inputmode="decimal"
      min={MIN_LB}
      max={MAX_LB}
      step="0.1"
      placeholder="—"
      value={shown}
      disabled={readonly || saving}
      onchange={commit}
      class="w-28 rounded-md border bg-transparent px-2 py-1.5 text-sm tabular-nums text-foreground outline-none transition focus:border-primary disabled:opacity-60 {rangeError
        ? 'border-destructive'
        : 'border-input'} {carried ? 'text-muted-foreground' : ''}"
    />
    <span class="text-sm text-muted-foreground">lb</span>
  </div>
  <p
    class="w-full text-xs {rangeError
      ? 'text-destructive'
      : 'text-muted-foreground'}"
    data-testid="bodyweight-caption"
  >
    {saving ? "Saving…" : caption}
  </p>
</Card>
