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
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import Plus from "@lucide/svelte/icons/plus";
  import SearchX from "@lucide/svelte/icons/search-x";
  import Trash2 from "@lucide/svelte/icons/trash-2";

  let exercises = $state<Exercise[]>([]);
  let loading = $state(true);
  let failed = $state(false);

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
    loading = true;
    failed = false;
    const { data, error } = await listExercises();
    if (error || !data) {
      failed = true;
      loading = false;
      return;
    }
    exercises = data;
    loading = false;
  }

  async function add(event: SubmitEvent) {
    event.preventDefault();
    const name = newName.trim();
    if (name === "" || saving) return;

    saving = true;
    actionError = null;
    const { data, error } = await createExercise({
      body: { name, muscleGroup: newGroup, equipment: newEquipment },
    });
    saving = false;
    if (error || !data) {
      actionError =
        error?.message ?? "Couldn't add that exercise. Try again.";
      return;
    }
    // Splice it in rather than refetching: the list is alphabetical, and one
    // insertion is cheaper and less jarring than a whole reload.
    exercises = [...exercises, data].sort((a, b) => a.name.localeCompare(b.name));
    newName = "";
    adding = false;
  }

  async function remove(exercise: Exercise) {
    actionError = null;
    const { error } = await deleteExercise({ path: { exerciseId: exercise.id } });
    if (error) {
      // The server's message names the reason — logged sets, or still on a
      // program — which is more use than "couldn't delete".
      actionError = error.message ?? `Couldn't delete ${exercise.name}.`;
      return;
    }
    exercises = exercises.filter((e) => e.id !== exercise.id);
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
    <Card class="h-64 animate-pulse" aria-hidden="true"></Card>
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
                <button
                  type="button"
                  class="shrink-0 rounded-md p-1.5 text-muted-foreground transition hover:text-destructive"
                  aria-label="Delete {exercise.name}"
                  onclick={() => remove(exercise)}
                >
                  <Trash2 class="size-4" aria-hidden="true" />
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
