<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";

  // The whole-page failure state, shown by ErrorBoundary when a route throws
  // while rendering. Unlike ErrorCard — which reports one page's data failing to
  // load and leaves the page around it intact — this stands in for the page.
  let {
    error,
    onReset,
  }: {
    error?: unknown;
    onReset?: () => void;
  } = $props();

  // The illustration is optional scenery: the heading carries the meaning, so a
  // missing file should cost nothing. Without this, a browser that can't fetch it
  // renders a broken-image icon on top of the error page, which reads as a second
  // failure.
  let showImage = $state(true);

  // Errors reach here as `unknown` — anything can be thrown. Show a message only
  // when there's a real one to show, rather than a bare "[object Object]".
  const detail = $derived(
    error instanceof Error
      ? error.message
      : typeof error === "string" && error.trim() !== ""
        ? error
        : "",
  );

  function goHome() {
    window.location.hash = "#/";
    // Navigation alone leaves the boundary in its failed state, so home would
    // render as this same error page. Reset so the new route gets to draw.
    onReset?.();
  }
</script>

<Card class="flex flex-col items-center gap-5 p-8 text-center" role="alert">
  {#if showImage}
    <img
      src="/images/gym-fire.png"
      alt="A gym engulfed in flames"
      class="w-full max-w-xs rounded-lg"
      onerror={() => (showImage = false)}
    />
  {/if}

  <div class="flex flex-col gap-2">
    <h2
      class="font-[var(--font-display)] text-3xl font-black uppercase tracking-[0.2em] text-neon drop-shadow-[0_0_18px_rgba(176,38,255,0.7)]"
    >
      Something went wrong
    </h2>
    <p class="text-sm text-muted-foreground">
      This page failed to load. Your logged sets are safe.
    </p>
  </div>

  {#if detail}
    <p class="max-w-md break-words font-mono text-xs text-muted-foreground/70">
      {detail}
    </p>
  {/if}

  <div class="flex flex-wrap justify-center gap-3">
    {#if onReset}
      <Button onclick={onReset}>Try again</Button>
    {/if}
    <Button variant="outline" onclick={goHome}>Go home</Button>
  </div>
</Card>
