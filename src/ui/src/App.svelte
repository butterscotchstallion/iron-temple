<script lang="ts">
  import RestTimer from "./lib/RestTimer.svelte";

  // Static placeholder mirroring the seeded programs. Phase 4 (part 2) will
  // replace this with the generated hey-api client (see openapi-ts.config.ts):
  //   import { listPrograms } from "./lib/api";
  const programs = [
    { name: "StrongLifts 5x5", blurb: "Squat, bench, row · A/B · 5×5" },
    { name: "StrongLifts 5x5 Lite", blurb: "Reduced volume · 2×5 per lift" },
    { name: "Advanced 3x5", blurb: "Graduation fork · 3×5" },
  ];
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
    {#each programs as program (program.name)}
      <article
        class="rounded-2xl border border-neon/30 bg-surface/70 p-5 shadow-[0_0_24px_-8px_rgba(176,38,255,0.6)] backdrop-blur transition hover:border-neon/70"
      >
        <h2 class="text-lg font-bold text-ink">{program.name}</h2>
        <p class="mt-1 text-sm text-ink/70">{program.blurb}</p>
      </article>
    {/each}
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
