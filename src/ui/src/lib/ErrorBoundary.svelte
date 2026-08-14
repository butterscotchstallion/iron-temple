<script lang="ts">
  import type { Snippet } from "svelte";
  import ErrorPage from "./ErrorPage.svelte";

  // Svelte's answer to a React error boundary. A component that throws while
  // rendering would otherwise tear down the whole app and leave a blank screen;
  // <svelte:boundary> confines the damage to this subtree and swaps in a fallback.
  //
  // Same limitation as React's: this catches errors thrown during render and
  // during effects. It does NOT catch throws inside event handlers, or rejected
  // promises from a fetch — those never pass through Svelte's rendering. Pages
  // keep handling their own load failures with ErrorCard; this is the net for
  // everything that isn't handled.
  let { children }: { children: Snippet } = $props();
</script>

<svelte:boundary
  onerror={(error) => {
    // The boundary swallows the error once it's handled, so without this it never
    // reaches the console and there's nothing to debug from.
    console.error("Unhandled render error:", error);
  }}
>
  {@render children()}

  {#snippet failed(error, reset)}
    <ErrorPage {error} onReset={reset} />
  {/snippet}
</svelte:boundary>
