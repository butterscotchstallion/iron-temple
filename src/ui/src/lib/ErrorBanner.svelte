<script lang="ts">
  import RotateCcw from "@lucide/svelte/icons/rotate-ccw";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import X from "@lucide/svelte/icons/x";

  // A slim, dismissible banner for action failures (a mutation or background
  // fetch that has no card of its own to fall back to). Optionally offers Retry.
  let {
    message,
    onRetry,
    onDismiss,
  }: {
    message: string;
    onRetry?: () => void;
    onDismiss?: () => void;
  } = $props();
</script>

<div
  class="flex items-center justify-between gap-3 rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive"
  role="alert"
>
  <span class="flex items-center gap-2">
    <TriangleAlert class="size-4 shrink-0" aria-hidden="true" />
    {message}
  </span>
  <div class="flex shrink-0 items-center gap-3">
    {#if onRetry}
      <button
        type="button"
        class="inline-flex items-center gap-1.5 font-semibold underline underline-offset-2 transition hover:opacity-80"
        onclick={onRetry}
      >
        <RotateCcw class="size-3.5" aria-hidden="true" />
        Retry
      </button>
    {/if}
    {#if onDismiss}
      <button
        type="button"
        class="transition hover:opacity-80"
        aria-label="Dismiss"
        onclick={onDismiss}
      >
        <X class="size-4" aria-hidden="true" />
      </button>
    {/if}
  </div>
</div>
