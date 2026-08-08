<script lang="ts">
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
  <span>{message}</span>
  <div class="flex shrink-0 items-center gap-3">
    {#if onRetry}
      <button
        type="button"
        class="font-semibold underline underline-offset-2 transition hover:opacity-80"
        onclick={onRetry}
      >
        Retry
      </button>
    {/if}
    {#if onDismiss}
      <button
        type="button"
        class="text-lg leading-none transition hover:opacity-80"
        aria-label="Dismiss"
        onclick={onDismiss}
      >
        ×
      </button>
    {/if}
  </div>
</div>
