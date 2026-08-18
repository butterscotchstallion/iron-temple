<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { getExerciseHistory, listExercises } from "../lib/api";
  import { exerciseEmoji, topSet } from "../lib/exerciseIcon";
  import { formatLongDate } from "../lib/date";
  import { Card } from "$lib/components/ui/card";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import ErrorBanner from "../lib/ErrorBanner.svelte";

  type ExerciseCard = {
    id: number;
    name: string;
    emoji: string;
    top: { weightLb: number; performedOn: string } | null;
  };

  let cards = $state<ExerciseCard[]>([]);
  let loading = $state(true);
  let failed = $state(false);
  // At least one lift's history request failed, so some cards are missing their
  // latest weight (and would otherwise read as "No sessions yet").
  let partialFailed = $state(false);

  // Placeholder cards shown while requests are in flight.
  const skeletons = [0, 1, 2, 3, 4, 5];

  async function load() {
    loading = true;
    failed = false;
    partialFailed = false;
    // Only the lifts with something to chart. Without the scope this asks for
    // the whole library — dozens of accessories nobody has touched — and then
    // fires a history request per card to discover they are empty. The Library
    // tab is where the full catalogue lives.
    const { data: exercises, error } = await listExercises({
      query: { scope: "performed" },
    });
    if (error || !exercises) {
      failed = true;
      loading = false;
      return;
    }
    // Fetch each exercise's history in parallel to compute its top set. A
    // single failing history request leaves that card without data, so flag it
    // rather than let the empty card read as "No sessions yet".
    cards = await Promise.all(
      exercises.map(async (exercise): Promise<ExerciseCard> => {
        const { data, error: historyError } = await getExerciseHistory({
          path: { exerciseId: exercise.id },
        });
        if (historyError) partialFailed = true;
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

  {#if partialFailed && !failed}
    <ErrorBanner
      message="Couldn't load some lifts' latest weight."
      onRetry={load}
      onDismiss={() => (partialFailed = false)}
    />
  {/if}

  <section class="grid gap-4 sm:grid-cols-3">
    {#if loading}
      {#each skeletons as n (n)}
        <Card class="h-40 animate-pulse" aria-hidden="true"></Card>
      {/each}
    {:else if failed}
      <ErrorCard
        class="col-span-full"
        message="Couldn't load exercises."
        onRetry={load}
      />
    {:else if cards.length === 0}
      <p class="col-span-full text-center text-sm text-muted-foreground">
        Nothing logged yet. Finish a workout and your lifts will show up here.
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
