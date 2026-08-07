<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { getExerciseHistory, listExercises } from "../lib/api";
  import { exerciseEmoji, topSet } from "../lib/exerciseIcon";
  import { formatLongDate } from "../lib/date";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";

  type ExerciseCard = {
    id: number;
    name: string;
    emoji: string;
    top: { weightLb: number; performedOn: string } | null;
  };

  let cards = $state<ExerciseCard[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  // Placeholder cards shown while requests are in flight.
  const skeletons = [0, 1, 2, 3, 4, 5];

  async function load() {
    loading = true;
    failed = false;
    const { data: exercises, error } = await listExercises();
    if (error || !exercises) {
      failed = true;
      loading = false;
      return;
    }
    // Fetch each exercise's history in parallel to compute its top set. A
    // single failing history request just leaves that card without data.
    cards = await Promise.all(
      exercises.map(async (exercise): Promise<ExerciseCard> => {
        const { data } = await getExerciseHistory({
          path: { exerciseId: exercise.id },
        });
        return {
          id: exercise.id,
          name: exercise.name,
          emoji: exerciseEmoji(exercise.name),
          top: data ? topSet(data) : null,
        };
      }),
    );
    loading = false;
  }

  onMount(load);
</script>

<div class="flex flex-col gap-4">
  <h2 class="text-2xl font-black text-foreground">Progress</h2>
  <p class="text-sm text-muted-foreground">Pick a lift to see how it's trending.</p>

  <section class="grid gap-4 sm:grid-cols-3">
    {#if loading}
      {#each skeletons as n (n)}
        <Card class="h-40 animate-pulse" aria-hidden="true"></Card>
      {/each}
    {:else if failed}
      <Card class="col-span-full p-6 text-center" role="alert">
        <p class="text-sm text-muted-foreground">Couldn't load exercises.</p>
        <Button variant="outline" class="mt-3" onclick={load}>Retry</Button>
      </Card>
    {:else if cards.length === 0}
      <p class="col-span-full text-center text-sm text-muted-foreground">
        No exercises yet.
      </p>
    {:else}
      {#each cards as card (card.id)}
        <a use:link href="/exercises/{card.id}" class="group block">
          <Card
            class="flex h-full flex-col items-center gap-1 p-5 text-center transition group-hover:ring-primary/60"
          >
            <span class="text-4xl" aria-hidden="true">{card.emoji}</span>
            <span class="font-bold text-card-foreground">{card.name}</span>
            {#if card.top}
              <span class="text-2xl font-black text-primary tabular-nums">
                {card.top.weightLb} lb
              </span>
              <span class="text-xs text-muted-foreground">
                {formatLongDate(card.top.performedOn)}
              </span>
            {:else}
              <span class="text-sm text-muted-foreground">No sessions yet</span>
            {/if}
          </Card>
        </a>
      {/each}
    {/if}
  </section>
</div>
