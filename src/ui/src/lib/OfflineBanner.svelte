<script lang="ts">
  import CloudOff from "@lucide/svelte/icons/cloud-off";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import { isOnline } from "./connectivity.svelte";
  import { clearRejected, queuedCount, rejectedCount } from "./writeQueue.svelte";

  // Three states worth saying out loud, and one worth saying nothing about.
  //
  // Offline is the headline: the lifter should know the app is not talking to
  // anything, precisely BECAUSE nothing on screen looks wrong — every tap still
  // registers. Told plainly, it reads as "carry on"; left unsaid, the sync that
  // happens later looks like a glitch.
  //
  // Back online with a queue still draining is worth a line too, because it is
  // the moment a lifter might close the tab believing everything is saved.
  //
  // A refused write is the only failure here. It is not transient and will not
  // retry, so it needs a dismissal rather than a spinner.
  //
  // Online with an empty queue renders nothing at all. A permanent "connected"
  // badge is noise on the one screen where every pixel is a thumb target.
  const online = $derived(isOnline());
  const waiting = $derived(queuedCount());
  const refused = $derived(rejectedCount());

  const changes = (n: number) => `${n} ${n === 1 ? "change" : "changes"}`;
</script>

{#if !online}
  <div
    class="border-b border-amber-400/30 bg-amber-500/15 px-5 py-2 text-center text-sm text-amber-200"
    role="status"
    aria-live="polite"
  >
    <span class="inline-flex items-center gap-2">
      <CloudOff class="size-4 shrink-0" aria-hidden="true" />
      {#if waiting > 0}
        Offline — {changes(waiting)} saved here, and sent when you're back.
      {:else}
        Offline — keep logging, it'll sync when you're back.
      {/if}
    </span>
  </div>
{:else if waiting > 0}
  <div
    class="border-b border-white/10 bg-white/5 px-5 py-2 text-center text-sm text-muted-foreground"
    role="status"
    aria-live="polite"
  >
    Syncing {changes(waiting)}…
  </div>
{/if}

{#if refused > 0}
  <div
    class="border-b border-destructive/40 bg-destructive/15 px-5 py-2 text-center text-sm text-destructive-foreground"
    role="alert"
  >
    <span class="inline-flex flex-wrap items-center justify-center gap-2">
      <TriangleAlert class="size-4 shrink-0" aria-hidden="true" />
      {changes(refused)} couldn't be saved.
      <button
        type="button"
        class="underline underline-offset-2 hover:no-underline"
        onclick={clearRejected}
      >
        Dismiss
      </button>
    </span>
  </div>
{/if}
