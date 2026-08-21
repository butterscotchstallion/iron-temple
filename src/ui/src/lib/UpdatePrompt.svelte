<script lang="ts">
  import Download from "@lucide/svelte/icons/download";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import { Button } from "$lib/components/ui/button";
  import { version, hasUpdate, dismissUpdate } from "./version.svelte";
  import { whenIdle } from "./pendingWrites.svelte";

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
