<script lang="ts">
  import { onMount } from "svelte";
  import { listPrograms, type ProgramSummary } from "./lib/api";
  import { programSubtitle } from "./lib/programs";
  import RestTimer from "./lib/RestTimer.svelte";

  let programs = $state<ProgramSummary[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  // Placeholder cards shown while the request is in flight.
  const skeletons = [0, 1, 2];

  async function load() {
    loading = true;
    failed = false;
    const { data, error } = await listPrograms();
    if (error || !data) {
      failed = true;
    } else {
      programs = data;
    }
    loading = false;
  }

  onMount(load);
</script>

<main class="mx-auto flex max-w-3xl flex-col gap-8 px-5 py-10">
  <header class="text-center">
    <h1
      class="font-[var(--font-display)] text-5xl font-black uppercase tracking-[0.2em] text-neon drop-shadow-[0_0_18px_rgba(176,38,255,0.7)] sm:text-6xl"
    >
      Iron Temple
    </h1>
    <p class="mt-2 text-sm uppercase tracking-[0.35em] text-cyan/80">
      Lift · Log · Progress
    </p>
  </header>

  <section class="grid gap-4 sm:grid-cols-3">
    {#if loading}
      {#each skeletons as n (n)}
        <article
          class="h-24 animate-pulse rounded-2xl border border-neon/20 bg-surface/50"
          aria-hidden="true"
        ></article>
      {/each}
    {:else if failed}
      <div
        class="col-span-full rounded-2xl border border-magenta/40 bg-surface/60 p-6 text-center"
        role="alert"
      >
        <p class="text-sm text-ink/80">Couldn't load programs.</p>
        <button
          class="mt-3 rounded-full border border-cyan/60 bg-cyan/10 px-5 py-2 font-semibold text-ink transition hover:bg-cyan/25"
          onclick={load}
        >
          Retry
        </button>
      </div>
    {:else if programs.length === 0}
      <p class="col-span-full text-center text-sm text-ink/60">
        No programs yet.
      </p>
    {:else}
      {#each programs as program (program.id)}
        <article
          class="rounded-2xl border border-neon/30 bg-surface/70 p-5 shadow-[0_0_24px_-8px_rgba(176,38,255,0.6)] backdrop-blur transition hover:border-neon/70"
        >
          <h2 class="text-lg font-bold text-ink">{program.name}</h2>
          <p class="mt-1 text-sm text-ink/70">{programSubtitle(program)}</p>
        </article>
      {/each}
    {/if}
  </section>

  <section
    class="rounded-2xl border border-cyan/30 bg-surface-2/60 p-6 backdrop-blur"
  >
    <h2 class="mb-4 text-center text-xs uppercase tracking-[0.3em] text-magenta">
      Rest Timer
    </h2>
    <RestTimer seconds={180} />
  </section>
</main>
