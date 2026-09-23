<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import {
    listExercises,
    createExercise,
    deleteExercise,
    type Exercise,
    type MuscleGroup,
    type Equipment,
  } from "../lib/api";
  import {
    EQUIPMENT,
    MUSCLE_GROUPS,
    countByGroup,
    equipmentLabel,
    exerciseSubtitle,
    groupExercises,
    muscleGroupLabel,
  } from "../lib/library";
  import { exerciseEmoji } from "../lib/exerciseIcon";
  import { CACHE_KEYS, cachedValue, fetchThrough, invalidate } from "../lib/cache.svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import Plus from "@lucide/svelte/icons/plus";
  import SearchX from "@lucide/svelte/icons/search-x";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import Loading from "../lib/skeleton/Loading.svelte";
  import Skeleton from "../lib/skeleton/Skeleton.svelte";

  let exercises = $state<Exercise[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  // Placeholder counts for the first paint. The seeded library is 53 movements
  // across nine groups, so two sections of five rows is about a phone screen —
  // enough to hold the fold still without reserving a page of empty scroll.
  const SKELETON_CHIPS = [0, 1, 2, 3, 4, 5];
  const SKELETON_SECTIONS = [0, 1];
  const SKELETON_ROWS = [0, 1, 2, 3, 4];

  // Search text and the muscle-group chip, if one is selected.
  let query = $state("");
  let group = $state<MuscleGroup | null>(null);

  // The "add your own" form, collapsed until asked for: the library is a place
  // to browse first and author second.
  let adding = $state(false);
  let newName = $state("");
  let newGroup = $state<MuscleGroup>("other");
  let newEquipment = $state<Equipment>("other");
  let saving = $state(false);
  // Which custom exercise is being deleted, if any.
  //
  // The delete was the one action on this page that ran silently: the trash
  // icon stayed live through the round trip, so a second tap sent a second
  // request for a row the first had already removed — and on a slow link the
  // first tap looked like it had done nothing at all.
  let removingId = $state<number | null>(null);
  // Message from a failed create or delete. A string rather than a boolean
  // because the API distinguishes a duplicate name from an exercise in use, and
  // that distinction is the whole value of the message.
  let actionError = $state<string | null>(null);

  const counts = $derived(countByGroup(exercises));
  const groups = $derived(groupExercises(exercises, { query, group }));
  const matchCount = $derived(
    groups.reduce((total, g) => total + g.exercises.length, 0),
  );

  async function load() {
    failed = false;

    const remembered = cachedValue<Exercise[]>(CACHE_KEYS.allExercises);
    if (remembered) exercises = remembered;
    loading = remembered === undefined;

    const result = await fetchThrough(CACHE_KEYS.allExercises, () => listExercises());
    if (result.status !== 200) {
      if (!remembered) failed = true;
    } else {
      exercises = result.data;
    }
    loading = false;
  }

  async function add(event: SubmitEvent) {
    event.preventDefault();
    const name = newName.trim();
    if (name === "" || saving) return;

    saving = true;
    actionError = null;
    const created = await createExercise({
      name,
      muscleGroup: newGroup,
      equipment: newEquipment,
    });
    saving = false;
    if (created.status !== 201) {
      actionError = created.data?.message ?? "Couldn't add that exercise. Try again.";
      return;
    }
    // Splice it in rather than refetching: the list is alphabetical, and one
    // insertion is cheaper and less jarring than a whole reload. The cached
    // copy is now a library short one movement, so drop it — the next visit
    // pays a load rather than opening on a list missing what was just added.
    exercises = [...exercises, created.data].sort((a, b) => a.name.localeCompare(b.name));
    invalidate(CACHE_KEYS.allExercises);
    newName = "";
    adding = false;
  }

  async function remove(exercise: Exercise) {
    if (removingId !== null) return;
    actionError = null;
    removingId = exercise.id;
    const deleted = await deleteExercise(exercise.id);
    removingId = null;
    if (deleted.status !== 204) {
      // The server's message names the reason — logged sets, or still on a
      // program — which is more use than "couldn't delete".
      actionError = deleted.data?.message ?? `Couldn't delete ${exercise.name}.`;
      return;
    }
    exercises = exercises.filter((e) => e.id !== exercise.id);
    invalidate(CACHE_KEYS.allExercises);
  }

  onMount(load);
</script>

<div class="flex flex-col gap-4">
  <div>
    <h2 class="text-2xl font-black text-foreground">Exercise library</h2>
    <p class="mt-1 text-sm text-muted-foreground">
      Every movement you can add to a program's assistance work.
    </p>
  </div>

  {#if actionError}
    <ErrorBanner message={actionError} onDismiss={() => (actionError = null)} />
  {/if}

  {#if loading}
    <!-- Three things go missing at once here, and the search box is the one
         worth naming: it is drawn from nothing the server sends, but it lives
         inside the loaded branch, so it used to appear with the catalogue and
         shove the whole list down a row. The chip row does the same. -->
    <Loading label="Loading the exercise library" class="flex flex-col gap-4">
      <Skeleton class="h-[38px] w-full" />
      <div class="flex flex-wrap gap-1.5">
        {#each SKELETON_CHIPS as chip (chip)}
          <Skeleton class="h-[26px] w-20 rounded-full" />
        {/each}
      </div>
      {#each SKELETON_SECTIONS as section (section)}
        <section class="flex flex-col gap-2">
          <Skeleton text="xs" class="w-24" />
          <Card class="divide-y divide-border/60 p-0">
            {#each SKELETON_ROWS as row (row)}
              <div class="flex items-center gap-3 px-4 py-2.5">
                <Skeleton text="xl" class="w-6 shrink-0" />
                <!-- Explicit heights rather than the `text` prop: the real name
                     and subtitle sit inside a `leading-tight` anchor, so their
                     line boxes are 1.25em rather than the type scale's default,
                     and the scale would overshoot the row by 8px. -->
                <div class="flex flex-1 flex-col">
                  <Skeleton class="h-[18px] w-40" />
                  <Skeleton class="h-[15px] w-28" />
                </div>
              </div>
            {/each}
          </Card>
        </section>
      {/each}
    </Loading>
  {:else if failed}
    <ErrorCard message="Couldn't load the exercise library." onRetry={load} />
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

    <!-- Counts come from the whole library, not the current search, so the
         chips don't collapse toward zero as you type. -->
    <div class="flex flex-wrap gap-1.5">
      <button
        type="button"
        aria-pressed={group === null}
        class="rounded-full border px-3 py-1 text-xs font-semibold uppercase tracking-wider transition {group ===
        null
          ? 'border-primary bg-primary text-primary-foreground'
          : 'border-border/60 text-muted-foreground hover:text-foreground'}"
        onclick={() => (group = null)}
      >
        All {exercises.length}
      </button>
      {#each MUSCLE_GROUPS as g (g)}
        {#if counts[g] > 0}
          <button
            type="button"
            aria-pressed={group === g}
            class="rounded-full border px-3 py-1 text-xs font-semibold uppercase tracking-wider transition {group ===
            g
              ? 'border-primary bg-primary text-primary-foreground'
              : 'border-border/60 text-muted-foreground hover:text-foreground'}"
            onclick={() => (group = group === g ? null : g)}
          >
            {muscleGroupLabel(g)}
            {counts[g]}
          </button>
        {/if}
      {/each}
    </div>

    {#if matchCount === 0}
      <Card class="flex flex-col items-center p-6 text-center">
        <SearchX class="size-8 text-muted-foreground/60" aria-hidden="true" />
        <p class="mt-3 text-sm text-muted-foreground">
          No exercises match that search.
        </p>
      </Card>
    {/if}

    {#each groups as section (section.group)}
      <section class="flex flex-col gap-2">
        <h3
          class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground"
        >
          {section.label}
        </h3>
        <Card class="divide-y divide-border/60 p-0">
          {#each section.exercises as exercise (exercise.id)}
            <div class="flex items-center gap-3 px-4 py-2.5">
              <!-- An emoji, not a form figure. The figures were tried here and
                   taken out: a stick figure needs room to read, and at the size
                   a list row can spare it is a smudge that costs the name its
                   space. They live on the lift's own page, where there is room
                   to see one. -->
              <span class="text-xl" aria-hidden="true">
                {exerciseEmoji(exercise.name)}
              </span>
              <a
                use:link
                href="/exercises/{exercise.id}"
                class="flex-1 leading-tight underline-offset-2 transition hover:text-primary hover:underline"
              >
                <span class="block font-semibold text-card-foreground">
                  {exercise.name}
                </span>
                <span class="block text-xs text-muted-foreground">
                  {exerciseSubtitle(exercise)}
                </span>
              </a>
              {#if exercise.isCustom}
                {@const removing = removingId === exercise.id}
                <!-- Disabled while ANY delete is in flight, not just this row's:
                     two of them racing is two requests against a list that is
                     about to be spliced twice. aria-busy carries the same fact
                     to a screen reader, which cannot see the icon dim. -->
                <button
                  type="button"
                  class="shrink-0 rounded-md p-1.5 text-muted-foreground transition hover:text-destructive disabled:opacity-40"
                  aria-label={removing
                    ? `Deleting ${exercise.name}`
                    : `Delete ${exercise.name}`}
                  aria-busy={removing}
                  disabled={removingId !== null}
                  onclick={() => remove(exercise)}
                >
                  <Trash2
                    class="size-4 {removing ? 'animate-pulse motion-reduce:animate-none' : ''}"
                    aria-hidden="true"
                  />
                </button>
              {/if}
            </div>
          {/each}
        </Card>
      </section>
    {/each}

    <!-- Authoring lives at the bottom: you scroll past the catalogue before
         concluding it's missing something. -->
    {#if adding}
      <Card class="p-5">
        <form class="flex flex-col gap-3" onsubmit={add}>
          <h3 class="font-bold text-card-foreground">Add your own exercise</h3>
          <label class="flex flex-col gap-1 text-sm">
            <span class="text-muted-foreground">Name</span>
            <input
              bind:value={newName}
              maxlength="80"
              required
              placeholder="Copenhagen Plank"
              class="rounded-md border border-input bg-transparent px-3 py-2 outline-none transition focus:border-primary"
            />
          </label>
          <div class="flex flex-wrap gap-3">
            <label class="flex flex-1 flex-col gap-1 text-sm">
              <span class="text-muted-foreground">Muscle group</span>
              <select
                bind:value={newGroup}
                class="rounded-md border border-input bg-transparent px-3 py-2 outline-none transition focus:border-primary"
              >
                {#each MUSCLE_GROUPS as g (g)}
                  <option value={g}>{muscleGroupLabel(g)}</option>
                {/each}
              </select>
            </label>
            <label class="flex flex-1 flex-col gap-1 text-sm">
              <span class="text-muted-foreground">Equipment</span>
              <select
                bind:value={newEquipment}
                class="rounded-md border border-input bg-transparent px-3 py-2 outline-none transition focus:border-primary"
              >
                {#each EQUIPMENT as eq (eq)}
                  <option value={eq}>{equipmentLabel(eq)}</option>
                {/each}
              </select>
            </label>
          </div>
          <div class="flex gap-2">
            <Button type="submit" size="sm" disabled={saving}>
              <Plus />
              {saving ? "Adding…" : "Add exercise"}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onclick={() => (adding = false)}
            >
              Cancel
            </Button>
          </div>
        </form>
      </Card>
    {:else}
      <Button
        variant="outline"
        size="sm"
        class="self-start"
        onclick={() => (adding = true)}
      >
        <Plus />
        Add your own exercise
      </Button>
    {/if}
  {/if}
</div>
