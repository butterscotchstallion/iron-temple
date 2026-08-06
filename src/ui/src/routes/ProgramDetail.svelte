<script lang="ts">
  // svelte-spa-router passes the matched route params as a `params` prop;
  // `id` comes from the `/programs/:id` pattern (a string from the URL).
  let { params }: { params?: { id?: string } } = $props();
</script>

<div class="flex flex-col gap-6">
  <a
    href="#/"
    class="text-sm uppercase tracking-[0.3em] text-cyan/80 transition hover:text-cyan"
  >
    ← Programs
  </a>

  <div class="rounded-2xl border border-neon/30 bg-surface/70 p-6 backdrop-blur">
    <p class="text-xs uppercase tracking-[0.3em] text-magenta">Program</p>
    <p class="mt-1 text-4xl font-black text-ink">#{params?.id}</p>
    <p class="mt-3 text-sm text-ink/60">
      Full detail — days, prescribed exercises, and progression-computed next
      weights — lands in the next slice.
    </p>
  </div>
</div>
