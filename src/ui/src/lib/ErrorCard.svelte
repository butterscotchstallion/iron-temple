<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";

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

<Card class="p-6 text-center {className}" role="alert">
  <p class="text-sm text-muted-foreground">{message}</p>
  {#if onRetry}
    <Button variant="outline" class="mt-3" onclick={onRetry}>Retry</Button>
  {/if}
</Card>
