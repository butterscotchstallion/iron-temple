<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { listExercises } from "../lib/api";
  import { exerciseEmoji } from "../lib/exerciseIcon";
  import { formatLongDate } from "../lib/date";
  import { Card } from "$lib/components/ui/card";
  import TrendingUp from "@lucide/svelte/icons/trending-up";
  import ErrorCard from "../lib/ErrorCard.svelte";

  type ExerciseCard = {
    id: number;
    name: string;
    emoji: string;
    top: { weightLb: number; performedOn: string } | null;
  };

  let cards = $state<ExerciseCard[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  // Placeholder cards shown while the request is in flight.
  const skeletons = [0, 1, 2, 3, 4, 5];

  async function load() {
    loading = true;
    failed = false;
    // One request for the whole page. The scope keeps it to lifts with
    // something to chart — the Library tab is where the full catalogue lives —
    // and each row now carries its own top set, so there is nothing left to
    // ask for. This used to be followed by a history request PER CARD, each
    // transferring a lift's entire history so the browser could take one
    // maximum from it; the server computes that maximum now.
    const { data: exercises, error } = await listExercises({
      query: { scope: "performed" },
    });
    if (error || !exercises) {
      failed = true;
      loading = false;
      return;
    }
    cards = exercises.map((exercise) => ({
      id: exercise.id,
      name: exercise.name,
      emoji: exerciseEmoji(exercise.name),
      top: exercise.topSet,
    }));
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
      <ErrorCard
        class="col-span-full"
        message="Couldn't load exercises."
        onRetry={load}
      />
    {:else if cards.length === 0}
      <div class="col-span-full flex flex-col items-center text-center">
        <TrendingUp class="size-8 text-muted-foreground/60" aria-hidden="true" />
        <p class="mt-3 text-sm text-muted-foreground">
          Nothing logged yet. Finish a workout and your lifts will show up here.
        </p>
      </div>
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
