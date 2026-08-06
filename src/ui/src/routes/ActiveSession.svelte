<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import {
    getSession,
    updateSessionSet,
    type Session,
    type SessionSet,
  } from "../lib/api";
  import RestTimer from "../lib/RestTimer.svelte";

  let { params }: { params?: { id?: string } } = $props();
  const sessionId = Number(params?.id);

  let session = $state<Session | null>(null);
  let loading = $state(true);
  let failed = $state(false);

  async function load() {
    loading = true;
    failed = false;
    const { data, error } = await getSession({ path: { sessionId } });
    if (error || !data) {
      failed = true;
    } else {
      session = data;
    }
    loading = false;
  }

  onMount(load);

  // Sets grouped by exercise, preserving prescription order.
  const groups = $derived.by(() => {
    const byExercise = new Map<string, SessionSet[]>();
    for (const set of session?.sets ?? []) {
      const list = byExercise.get(set.exerciseName) ?? [];
      list.push(set);
      byExercise.set(set.exerciseName, list);
    }
    return [...byExercise.entries()].map(([name, sets]) => ({ name, sets }));
  });

  const completedCount = $derived(
    (session?.sets ?? []).filter((s) => s.completed).length,
  );

  // Toggle a set complete/incomplete; on completion, log the target reps.
  async function toggle(set: SessionSet) {
    const completed = !set.completed;
    const { data } = await updateSessionSet({
      path: { sessionId, setId: set.id },
      body: { completed, actualReps: completed ? set.targetReps : null },
    });
    if (data && session) {
      session.sets = session.sets.map((s) => (s.id === data.id ? data : s));
    }
  }
</script>

<div class="flex flex-col gap-6">
  <a
    href="/"
    use:link
    class="text-sm uppercase tracking-[0.3em] text-cyan/80 transition hover:text-cyan"
  >
    ← Programs
  </a>

  {#if loading}
    <div class="h-40 animate-pulse rounded-2xl border border-neon/20 bg-surface/50"></div>
  {:else if failed}
    <div
      class="rounded-2xl border border-magenta/40 bg-surface/60 p-6 text-center"
      role="alert"
    >
      <p class="text-sm text-ink/80">Couldn't load this session.</p>
      <button
        class="mt-3 rounded-full border border-cyan/60 bg-cyan/10 px-5 py-2 font-semibold text-ink transition hover:bg-cyan/25"
        onclick={load}
      >
        Retry
      </button>
    </div>
  {:else if session}
    <header>
      <h2 class="text-2xl font-black text-ink">{session.programName}</h2>
      <p class="mt-1 text-sm text-ink/70">
        {session.programDayName} · {session.performedOn}
      </p>
      <p class="mt-1 text-xs uppercase tracking-[0.3em] text-cyan/80">
        {completedCount} / {session.sets.length} sets done
      </p>
    </header>

    <section class="rounded-2xl border border-cyan/30 bg-surface-2/60 p-6 backdrop-blur">
      <h3 class="mb-4 text-center text-xs uppercase tracking-[0.3em] text-magenta">
        Rest Timer
      </h3>
      <RestTimer seconds={180} />
    </section>

    {#each groups as group (group.name)}
      <section class="rounded-2xl border border-neon/30 bg-surface/70 p-5 backdrop-blur">
        <h3 class="text-lg font-bold text-ink">{group.name}</h3>
        <div class="mt-3 flex flex-col gap-2">
          {#each group.sets as set (set.id)}
            <button
              class="flex items-center justify-between rounded-xl border px-4 py-2 text-left text-sm transition {set.completed
                ? 'border-cyan/60 bg-cyan/15 text-ink'
                : 'border-neon/20 bg-surface-2/40 text-ink/80 hover:border-neon/50'}"
              onclick={() => toggle(set)}
            >
              <span>Set {set.setNumber} · {set.targetReps} reps · {set.weightLb} lb</span>
              <span class="text-lg">{set.completed ? "✓" : "○"}</span>
            </button>
          {/each}
        </div>
      </section>
    {/each}
  {/if}
</div>
