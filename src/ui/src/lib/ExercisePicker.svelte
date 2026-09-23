<script lang="ts">
  import { onMount } from "svelte";
  import { listExercises, type Exercise, type MuscleGroup } from "./api";
  import {
    MUSCLE_GROUPS,
    countByGroup,
    exerciseSubtitle,
    groupExercises,
    muscleGroupLabel,
    recentExercises,
  } from "./library";
  import { exerciseEmoji } from "./exerciseIcon";
  import { Button } from "$lib/components/ui/button";
  import ErrorBanner from "./ErrorBanner.svelte";
  import Loading from "./skeleton/Loading.svelte";
  import Skeleton from "./skeleton/Skeleton.svelte";
  import SearchX from "@lucide/svelte/icons/search-x";

  // Choosing a movement from the library: search, muscle-group chips, the
  // lifter's recent lifts, and the catalogue grouped under headings.
  //
  // Extracted from AssistancePicker, which was two panels in one component —
  // this half, then a form prescribing what was chosen. Two callers now want the
  // first half and disagree entirely about the second: assistance prescribes a
  // rep range and a carried-forward weight, while a program's own prescription
  // asks where the lift STARTS, a number consulted only until there is history.
  //
  // Extracted rather than given a mode flag, for that reason. The two step-twos
  // share no field whose meaning is the same, so a flag would be two components
  // in a trench coat with one set of state.
  //
  // Deliberately not a dialog: choosing a lift means scrolling a long list
  // against the day you are adding it to, and a modal covering the day takes
  // that context away.
  let {
    exclude = [],
    onPick,
    onCancel,
  }: {
    /** Exercise ids already on this day, which offering again would only 409. */
    exclude?: number[];
    onPick: (exercise: Exercise) => void;
    onCancel: () => void;
  } = $props();

  // Chips in the loading placeholder — one per muscle group with anything in
  // it, which for the seeded library is most of the nine.
  const SKELETON_CHIPS = [0, 1, 2, 3, 4, 5, 6, 7];

  let exercises = $state<Exercise[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  let query = $state("");
  let group = $state<MuscleGroup | null>(null);

  const available = $derived(exercises.filter((e) => !exclude.includes(e.id)));
  const counts = $derived(countByGroup(available));
  const groups = $derived(groupExercises(available, { query, group }));
  const matchCount = $derived(
    groups.reduce((total, g) => total + g.exercises.length, 0),
  );

  // The lifts this lifter actually trains, so the handful they always reach for
  // sit above the catalogue instead of behind a search for the same names every
  // time. Derived from `available`, so anything already on the day is out of it
  // for free.
  //
  // Only while the list is unfiltered. Once a search or a chip is on, the list
  // below is already short and this stops being a shortcut to it — it becomes a
  // duplicate sitting between a lifter and the match they typed for.
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

  onMount(load);
</script>

{#if loading}
  <!-- The picker's own shape: the search box, the muscle-group chips and the
       scrolling list, which is capped at max-h-64 whatever comes back. A single
       h-24 block used to stand in for all three, so the panel grew by most of
       its own height when the library landed — inside a dialog or an expanded
       card, which pushed whatever was under it down the page. -->
  <Loading label="Loading the exercise library" class="flex flex-col gap-3">
    <Skeleton class="h-[38px] w-full" />
    <div class="flex flex-wrap gap-1.5">
      {#each SKELETON_CHIPS as chip (chip)}
        <Skeleton class="h-[22px] w-16 rounded-full" />
      {/each}
    </div>
    <Skeleton class="h-64 w-full" />
    <!-- Cancel, which the loaded branch draws whatever came back. -->
    <Skeleton class="h-8 w-20 self-start rounded-md" />
  </Loading>
{:else if failed}
  <ErrorBanner message="Couldn't load the exercise library." onRetry={load} />
{:else}
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
           Removing them from it would make a movement go missing from the one
           place a lifter scrolls to expecting it. -->
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
    onclick={() => onPick(exercise)}
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
