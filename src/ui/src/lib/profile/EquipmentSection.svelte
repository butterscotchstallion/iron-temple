<script lang="ts">
  import { untrack } from "svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import ErrorBanner from "../ErrorBanner.svelte";
  import { setMe } from "../auth.svelte";
  import { updateMe, type User } from "../api";
  import { fieldClass, labelClass } from "./fieldStyles";

  // The gym: what the bar weighs and what is in the rack. Both used to be
  // constants in the bundle, and both were wrong — the bar was assumed to be
  // 45 lb, and the plate set was treated as unlimited. Every weight the app
  // draws is loaded onto them, so they belong to the lifter, not to the build.
  let { me }: { me: User } = $props();

  // Local copies, untracked so they capture the gym as it stood when the
  // section mounted: edits should be discardable by navigating away, and
  // binding straight to auth.me would rewrite the profile as the lifter typed.
  let barWeight = $state(untrack(() => me.barWeightLb) ?? 45);
  let plates = $state<{ plateLb: number; pairs: number }[]>(
    untrack(() => me.plates ?? []).map((p) => ({ ...p })),
  );
  // What the dumbbell rack steps by, per bell — the third thing the gym is made
  // of, and the one that was still a constant in the engine until 0020. A pair
  // moves twice this, so a rack of 5s has nothing between 10 lb and a rack of
  // 2.5s has nothing between 5.
  let dumbbellStep = $state(untrack(() => me.dumbbellStepLb) ?? 5);
  let gymSaving = $state(false);
  let gymError = $state<string | null>(null);
  let gymSaved = $state(false);

  // The denominations a rack is built from. Owning none of one is normal, so
  // this is the menu rather than a claim about what is there.
  const DENOMINATIONS = [45, 35, 25, 10, 5, 2.5];

  function pairsOf(plateLb: number): number {
    return plates.find((p) => p.plateLb === plateLb)?.pairs ?? 0;
  }

  function setPairs(plateLb: number, pairs: number) {
    const next = Math.max(0, Math.min(20, pairs));
    const existing = plates.find((p) => p.plateLb === plateLb);
    // Zero pairs is an absent row, not a row saying zero — the API rejects
    // pairs < 1, and "I own none" is expressed by not listing it.
    if (next === 0) {
      plates = plates.filter((p) => p.plateLb !== plateLb);
    } else if (existing) {
      existing.pairs = next;
    } else {
      plates = [...plates, { plateLb, pairs: next }].sort(
        (a, b) => b.plateLb - a.plateLb,
      );
    }
  }

  async function saveGym(event: SubmitEvent) {
    event.preventDefault();
    gymSaving = true;
    gymError = null;
    gymSaved = false;

    const saved = await updateMe({
      barWeightLb: barWeight,
      dumbbellStepLb: dumbbellStep,
      plates,
    });
    if (saved.status !== 200) {
      gymError = "Couldn't save your gym setup.";
    } else {
      setMe(saved.data);
      gymSaved = true;
    }
    gymSaving = false;
  }
</script>

<Card class="p-6">
  <h3 class="text-lg font-bold text-card-foreground">Equipment</h3>
  <p class="mt-1 text-sm text-muted-foreground">
    Every weight the app shows gets loaded onto this bar with these plates. If
    they're wrong, the plate maths and the warm-ups are wrong with them.
  </p>
  <form class="mt-4 flex flex-col gap-4" onsubmit={saveGym}>
    {#if gymError}
      <ErrorBanner message={gymError} onDismiss={() => (gymError = null)} />
    {/if}

    <label class="flex flex-col gap-1.5">
      <span class={labelClass}>Bar weight (lb)</span>
      <input
        bind:value={barWeight}
        type="number"
        min="1"
        max="200"
        step="0.5"
        required
        class="{fieldClass} w-32"
      />
      <span class="text-xs text-muted-foreground">
        A standard Olympic bar is 45 lb. Weigh yours if you're not sure.
      </span>
    </label>

    <label class="flex flex-col gap-1.5">
      <span class={labelClass}>Dumbbell step (lb per bell)</span>
      <input
        bind:value={dumbbellStep}
        type="number"
        min="1"
        max="25"
        step="0.25"
        required
        class="{fieldClass} w-32"
      />
      <!-- Spelled out in both units on purpose. The number asked for is the
           one written on the rack, but every weight the app shows is the
           pair, so a lifter typing 5 should see the 10 it becomes rather
           than discover it when a curl jumps further than they expected. -->
      <span class="text-xs text-muted-foreground">
        The gap between one bell and the next. Most racks step 5 lb; adjustable
        dumbbells often do 2.5. A pair moves twice this, so yours go up
        {dumbbellStep * 2} lb at a time.
      </span>
    </label>

    <fieldset class="flex flex-col gap-2">
      <legend class={labelClass}>Plates you own (pairs)</legend>
      <p class="text-xs text-muted-foreground">
        Pairs, not plates — one per side. Set a plate to 0 if you don't have
        any; the app won't ask you to load it.
      </p>
      <div class="mt-1 flex flex-col gap-2">
        {#each DENOMINATIONS as plateLb (plateLb)}
          <div class="flex items-center gap-3">
            <span class="w-16 text-sm font-bold tabular-nums text-card-foreground">
              {plateLb} lb
            </span>
            <input
              type="number"
              min="0"
              max="20"
              value={pairsOf(plateLb)}
              oninput={(e) => setPairs(plateLb, Number(e.currentTarget.value))}
              aria-label={`Pairs of ${plateLb} lb plates`}
              class="{fieldClass} w-20"
            />
          </div>
        {/each}
      </div>
    </fieldset>

    <div class="flex items-center gap-3">
      <Button type="submit" disabled={gymSaving}>
        {gymSaving ? "Saving…" : "Save"}
      </Button>
      {#if gymSaved}
        <span class="text-sm text-muted-foreground" role="status">Saved.</span>
      {/if}
    </div>
  </form>
</Card>
