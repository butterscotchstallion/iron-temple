<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import CircleAlert from "@lucide/svelte/icons/circle-alert";
  import RotateCcw from "@lucide/svelte/icons/rotate-ccw";

  // A full-width failure card for when a page's data can't be loaded. Shows a
  // muted message and, when a retry handler is supplied, a Retry button.
  let {
    message,
    onRetry,
    class: className = "",
  }: {
    message: string;
    onRetry?: () => void;
    class?: string;
  } = $props();
</script>

<Card class="flex flex-col items-center p-6 text-center {className}" role="alert">
  <CircleAlert class="size-6 text-muted-foreground" aria-hidden="true" />
  <p class="mt-2 text-sm text-muted-foreground">{message}</p>
  {#if onRetry}
    <Button variant="outline" class="mt-3" onclick={onRetry}>
      <RotateCcw />
      Retry
    </Button>
  {/if}
</Card>
