<script lang="ts">
  import LoaderCircle from "@lucide/svelte/icons/loader-circle";
  import X from "@lucide/svelte/icons/x";
  import { dismissToast, toasts } from "./toast.svelte";

  // Where transient notices are drawn. The store decides what and for how long;
  // this only positions and styles them — see toast.svelte.ts.
  //
  // Mounted once, by the shell, outside the ErrorBoundary: a route that throws is
  // one of the things most worth saying out loud, so the region that says it must
  // not be inside what fell over.
  //
  // Fixed to the bottom rather than under the header, which is where the other two
  // banners live. Those report on the whole app and belong in its chrome; a toast
  // reports on something you just did, and pushing the page down to announce it
  // would move the button under the thumb that pressed it.
  const tones: Record<string, string> = {
    info: "border-border/60 bg-card text-foreground",
    success: "border-primary/40 bg-primary/10 text-foreground",
    danger: "border-destructive/40 bg-destructive/15 text-destructive-foreground",
  };
</script>

<!-- One live region holding all of them, rather than one per toast. A screen
     reader should hear the notices as they arrive, in order; `aria-live` on each
     box would announce a fresh region every time.

     polite, not assertive: nothing raised here is urgent enough to cut across
     what is already being read. -->
<div
  class="pointer-events-none fixed inset-x-0 bottom-0 z-50 flex flex-col items-center gap-2 p-4 sm:items-end"
  role="status"
  aria-live="polite"
  data-testid="toaster"
>
  {#each toasts() as toast (toast.id)}
    <!-- pointer-events re-enabled per toast: the container spans the viewport so
         the stack can align itself, and left clickable it would swallow taps on
         everything behind it. -->
    <div
      class="pointer-events-auto flex w-full max-w-sm items-start gap-3 rounded-md border px-4 py-3 shadow-lg shadow-black/40 {tones[
        toast.tone
      ]}"
    >
      {#if toast.pending}
        <LoaderCircle class="mt-0.5 size-4 shrink-0 animate-spin opacity-70" aria-hidden="true" />
      {/if}
      <div class="min-w-0 flex-1">
        <p class="text-sm font-semibold">{toast.title}</p>
        {#if toast.body}
          <p class="mt-0.5 text-xs text-muted-foreground">{toast.body}</p>
        {/if}
      </div>
      <!-- No dismiss on a pending notice: there is nothing to dismiss yet, and a
           reader who took it down would be left with a screen that looks idle
           while the request is still open. It goes when it resolves. -->
      {#if !toast.pending}
        <button
          type="button"
          class="-mr-1 -mt-1 rounded-sm p-1 opacity-60 transition hover:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          onclick={() => dismissToast(toast.id)}
          aria-label="Dismiss"
        >
          <X class="size-3.5" aria-hidden="true" />
        </button>
      {/if}
    </div>
  {/each}
</div>
