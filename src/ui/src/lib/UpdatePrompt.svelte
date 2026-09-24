<script lang="ts">
  import Download from "@lucide/svelte/icons/download";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import { Button } from "$lib/components/ui/button";
  import { version, hasUpdate, dismissUpdate, updateNotes } from "./version.svelte";
  import { whenIdle } from "./pendingWrites.svelte";
  import ChangelogList from "./ChangelogList.svelte";

  // "A new version is available — load it?" A static bundle behind nginx has no
  // way to update itself, so without this a lifter can sit on a build for days
  // and only ever get the new one by chance.
  //
  // The reason this can interrupt a workout at all is that taking it is free:
  // the active session logs every set to the server as it happens, the hash
  // route survives a reload, the rest countdown is persisted (restStorage.ts),
  // and the button below waits for any request still in the air before it
  // reloads (pendingWrites.svelte.ts). The copy says so, because "reload now"
  // mid-session reads as a threat otherwise.

  let {
    // The reload itself, injectable so tests can assert on it — jsdom has no
    // navigation and location.reload isn't stubbable in place.
    reload = () => window.location.reload(),
  }: { reload?: () => void } = $props();

  let open = $state(false);
  // The gap between pressing the button and the page going away, which exists
  // only for as long as a write is still settling.
  let updating = $state(false);

  // What shipped in the build on offer, so "Load it?" is a question with enough
  // information to answer. Fetched from the deployed bundle rather than baked in
  // — this one only knows what's in itself — so it can arrive a moment after the
  // dialog does, or not at all. `$derived` because of the former, `{#if}` in the
  // markup because of the latter.
  const notes = $derived(updateNotes());
  const notesId = $props.id();

  // Raise the dialog when a poll finds a newer build. Not a two-way binding on
  // hasUpdate(): the lifter closing it must not un-deploy the release, so the
  // open flag is ours and dismissal is recorded separately.
  $effect(() => {
    if (hasUpdate()) open = true;
  });

  // Escape, the overlay and "Not now" are the same intent, so they get the same
  // answer: stop asking about *this* version. A later release moves `latest`
  // past what was dismissed and the dialog comes back on its own.
  function onOpenChange(next: boolean) {
    if (next || updating) return;
    dismissUpdate();
  }

  async function apply() {
    if (updating) return;
    updating = true;

    // Commit anything half-typed before the page goes. The bodyweight box saves
    // on change, so a value typed but not blurred is unsaved — opening the
    // dialog already moves focus and fires it, and this covers the case where
    // it didn't.
    (document.activeElement as HTMLElement | null)?.blur();

    // Let the writes land. Capped inside whenIdle(), so a request that never
    // comes back delays the reload rather than cancelling it.
    await whenIdle();
    reload();
  }
</script>

<AlertDialog.Root bind:open {onOpenChange}>
  <AlertDialog.Content data-testid="update-prompt">
    <AlertDialog.Header>
      <AlertDialog.Title class="flex items-center gap-2">
        <Download class="size-5 shrink-0" aria-hidden="true" />
        New version available
      </AlertDialog.Title>
      <AlertDialog.Description>
        Iron Temple {version.latest} has been deployed — you're on {version.running}.
        Loading it reloads the app.
      </AlertDialog.Description>
    </AlertDialog.Header>

    {#if notes.length > 0}
      <!-- Capped in viewport units: AlertDialog.Content sets no max-height and is
           centred with -translate-y-1/2, so a release with a long list of notes
           would push "Load it" off the bottom of a landscape phone. Containing
           the overscroll stops a flick that reaches the end of this list from
           scrolling the page underneath the dialog. -->
      <section aria-labelledby={notesId} class="max-h-[35vh] overflow-y-auto overscroll-contain">
        <!-- Worded exactly as the header panel's heading, so the two places that
             show release notes read as one feature rather than two. h3 because
             AlertDialog.Title renders a div with role="heading" aria-level="2",
             so this is the level below it and the outline stays in order. -->
        <h3 id={notesId} class="text-[0.65rem] font-semibold uppercase tracking-[0.25em] text-primary">
          What's new in {version.latest}
        </h3>
        <ChangelogList entries={notes} class="mt-3" />
      </section>
    {/if}

    <p class="text-sm text-muted-foreground">
      Your workout is safe: every set you've logged is already saved, and the
      rest timer picks up where it left off.
    </p>

    <AlertDialog.Footer>
      <AlertDialog.Cancel disabled={updating}>Not now</AlertDialog.Cancel>
      <!-- A plain Button rather than AlertDialog.Action: Action closes the
           dialog on click, and this one has to stay up while the last write
           settles so there's something to show for the wait. -->
      <Button onclick={apply} disabled={updating}>
        {updating ? "Saving…" : "Load it"}
      </Button>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
