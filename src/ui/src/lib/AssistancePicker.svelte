<script lang="ts">
  import { onMount } from "svelte";
  import { listExercises, type Exercise, type MuscleGroup } from "./api";
  import {
    MUSCLE_GROUPS,
    countByGroup,
    equipmentStepLb,
    exerciseSubtitle,
    groupExercises,
    muscleGroupLabel,
    recentExercises,
  } from "./library";
  import { exerciseEmoji } from "./exerciseIcon";
  import { Button } from "$lib/components/ui/button";
  import ErrorBanner from "./ErrorBanner.svelte";
  import Plus from "@lucide/svelte/icons/plus";
  import SearchX from "@lucide/svelte/icons/search-x";

  // Picking an exercise from the library and prescribing it, in one panel.
  //
  // Deliberately not an AlertDialog: choosing assistance means scrolling a long
  // list against the day you are adding it to, and a modal that covers the day
  // takes that context away. It expands in place instead.
  let {
    exclude = [],
    onAdd,
    onCancel,
  }: {
    // Exercise ids already on this day — one entry per lift, so offering them
    // again would only earn a 409.
    exclude?: number[];
    onAdd: (choice: {
      exerciseId: number;
      sets: number;
      reps: number;
      weightLb: number;
      repMin?: number;
      repMax?: number;
    }) => Promise<boolean>;
    onCancel: () => void;
  } = $props();

  let exercises = $state<Exercise[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  let query = $state("");
  let group = $state<MuscleGroup | null>(null);
  let selected = $state<Exercise | null>(null);

  // Three sets of ten at bodyweight: the default nearly every accessory starts
  // at, and all three are editable before adding.
  let sets = $state(3);
  let reps = $state(10);
  let weightLb = $state(0);
  let saving = $state(false);

  // Off by default. A rep range turns the lift onto double progression — add
  // reps inside the range week to week, and when every set reaches the top the
  // weight goes up by one step of the equipment and the reps reset to the
  // bottom. Without it the lift carries its weight forward and nothing moves it,
  // which is what accessories have always done here and is still the right
  // default for most of them.
  let ranged = $state(false);
  let repMin = $state(8);
  let repMax = $state(12);

  // What the chosen movement's weight moves in: 5 lb on a bar, 10 on a pair of
  // dumbbells. Drives both the copy below and the number input's step, so the
  // arrows offer weights the rack can actually make.
  const stepLb = $derived(selected ? equipmentStepLb(selected.equipment) : 5);

  const available = $derived(exercises.filter((e) => !exclude.includes(e.id)));
  const counts = $derived(countByGroup(available));
  const groups = $derived(groupExercises(available, { query, group }));
  const matchCount = $derived(
    groups.reduce((total, g) => total + g.exercises.length, 0),
  );

  // The accessories this lifter actually trains, so the six they always add sit
  // above the catalogue instead of behind a search for the same names every
  // time. Derived from `available`, so everything already on this day is out of
  // it for free.
  //
  // Only while the list is unfiltered. Once a search or a group chip is on, the
  // list below is already short and the section is no longer a shortcut to it —
  // it is a duplicate sitting between a lifter and the match they typed for.
  const recent = $derived(recentExercises(available));
  const showRecent = $derived(
    recent.length > 0 && query.trim() === "" && group === null,
  );

  async function load() {
    loading = true;
    failed = false;
    const result = await listExercises();
    if (result.status !== 200) {
      failed = true;
      loading = false;
      return;
    }
    exercises = result.data;
    loading = false;
  }

  async function confirm() {
    if (!selected || saving) return;
    saving = true;
    const ok = await onAdd({
      exerciseId: selected.id,
      sets,
      // With a range the bottom is the rep target: a set is complete at the
      // bottom and the weight moves at the top.
      reps: ranged ? repMin : reps,
      weightLb,
      ...(ranged ? { repMin, repMax } : {}),
    });
    saving = false;
    // On failure the parent shows the banner and the panel stays open with the
    // selection intact, so the numbers needn't be typed twice.
    if (ok) selected = null;
  }

  onMount(load);
</script>

<div class="mt-3 flex flex-col gap-3 rounded-md border border-border/60 p-3">
  {#if loading}
    <div class="h-24 animate-pulse rounded-md bg-muted/40" aria-hidden="true"></div>
  {:else if failed}
    <ErrorBanner message="Couldn't load the exercise library." onRetry={load} />
  {:else if selected}
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
        Weight (lb)
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
    <p class="text-xs text-muted-foreground">
      Leave the weight at 0 for bodyweight work.
      {#if ranged}
        With a range, hit the top on every set and the weight goes up {stepLb} lb
        next time, with the reps back at the bottom. It never deloads.
      {:else}
        After the first time you log it, the weight carries over from your last
        session — nothing moves it but you.
      {/if}
    </p>
    <div class="flex gap-2">
      <Button size="sm" onclick={confirm} disabled={saving}>
        <Plus />
        {saving ? "Adding…" : "Add to this day"}
      </Button>
      <Button size="sm" variant="ghost" onclick={onCancel}>Cancel</Button>
    </div>
  {:else}
    <!-- Step one: choose the movement. -->
    <label class="flex flex-col gap-1">
      <span class="sr-only">Search exercises</span>
      <input
        type="search"
        bind:value={query}
        placeholder="Search exercises…"
        class="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none transition focus:border-primary"
      />
    </label>

    <div class="flex flex-wrap gap-1.5">
      {#each MUSCLE_GROUPS as g (g)}
        {#if counts[g] > 0}
          <button
            type="button"
            aria-pressed={group === g}
            class="rounded-full border px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-wider transition {group ===
            g
              ? 'border-primary bg-primary text-primary-foreground'
              : 'border-border/60 text-muted-foreground hover:text-foreground'}"
            onclick={() => (group = group === g ? null : g)}
          >
            {muscleGroupLabel(g)}
          </button>
        {/if}
      {/each}
    </div>

    <div class="max-h-64 overflow-y-auto rounded-md border border-border/40">
      {#if matchCount === 0}
        <div class="flex flex-col items-center p-4 text-center">
          <SearchX class="size-6 text-muted-foreground/60" aria-hidden="true" />
          <p class="mt-2 text-sm text-muted-foreground">
            No exercises match that search.
          </p>
        </div>
      {/if}
      {#if showRecent}
        <!-- Recent lifts stay listed under their muscle group below as well.
             Removing them from it would make a movement go missing from the
             one place a lifter scrolls to expecting it. -->
        {@render sectionHeading("Recent")}
        {#each recent as exercise (exercise.id)}
          {@render exerciseRow(exercise)}
        {/each}
      {/if}
      {#each groups as section (section.group)}
        {@render sectionHeading(section.label)}
        {#each section.exercises as exercise (exercise.id)}
          {@render exerciseRow(exercise)}
        {/each}
      {/each}
    </div>

    <Button size="sm" variant="ghost" class="self-start" onclick={onCancel}>
      Cancel
    </Button>
  {/if}
</div>

<!-- One row and one heading, rendered by both the Recent section and the muscle
     groups, so the two cannot drift apart. -->
{#snippet sectionHeading(label: string)}
  <p
    class="sticky top-0 bg-card px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground"
  >
    {label}
  </p>
{/snippet}

{#snippet exerciseRow(exercise: Exercise)}
  <button
    type="button"
    class="flex w-full items-center gap-2 px-3 py-2 text-left transition hover:bg-foreground/5"
    onclick={() => (selected = exercise)}
  >
    <span class="text-base" aria-hidden="true">
      {exerciseEmoji(exercise.name)}
    </span>
    <span class="flex-1 leading-tight">
      <span class="block text-sm font-medium text-card-foreground">
        {exercise.name}
      </span>
      <span class="block text-[11px] text-muted-foreground">
        {exerciseSubtitle(exercise)}
      </span>
    </span>
  </button>
{/snippet}
