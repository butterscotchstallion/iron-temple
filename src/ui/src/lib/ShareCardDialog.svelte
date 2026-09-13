<script lang="ts">
  import { onDestroy } from "svelte";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import { Button } from "$lib/components/ui/button";
  import { SHARE_CARD, type ShareCardContent } from "./shareCard";
  import { canShareFile, renderShareCard, shareCardFile, shareOrDownload } from "./shareImage";

  // The share preview.
  //
  // The image shown here is the file that will be sent — the same blob, by
  // object URL, not a second rendering of the same data. A preview that is
  // merely a faithful reproduction can still be wrong about the thing you are
  // about to post; this one cannot be.
  //
  // Rendered on open and discarded on close rather than cached, so switching the
  // page between month and year can never leave last period's card behind the
  // button. Painting costs a few milliseconds.
  //
  // Takes prepared content rather than a report: the month's recap and a single
  // workout's both come here, and everything below — the token dance, the object
  // URL, the share-vs-download fork — is the same for either. What differs is
  // only which selector filled the content in.

  let {
    open = $bindable(false),
    content,
    filename,
    alt,
    title = "Share your recap",
    subtitle,
  }: {
    open?: boolean;
    content: ShareCardContent;
    /** What the file is called once saved. */
    filename: string;
    /** Alt text for the preview — what the card says, for a reader who can't see it. */
    alt: string;
    title?: string;
    subtitle: string;
  } = $props();

  type Status = "rendering" | "ready" | "failed";

  let status = $state<Status>("rendering");
  let previewUrl = $state("");
  let file = $state<File | null>(null);
  let busy = $state(false);
  let problem = $state("");

  // Deliberately not $state: revoke() reads objectUrl and runs inside the effect
  // below, and a reactive read there would make the effect depend on a value it
  // also writes, which is a loop. `token` is likewise plain — it exists to be
  // compared, never to trigger anything.
  let objectUrl = "";
  let token = 0;

  /** Release the URL the preview holds. Touches nothing reactive. */
  function revoke() {
    if (objectUrl) URL.revokeObjectURL(objectUrl);
    objectUrl = "";
  }

  function discard() {
    revoke();
    previewUrl = "";
    file = null;
    problem = "";
  }

  async function render() {
    discard();
    status = "rendering";

    // Painting is asynchronous, and both closing the dialog and switching the
    // page between month and year can happen while it is in flight. A render
    // that is no longer the current one drops its result instead of writing
    // last period's card over this one's — or over a destroyed component.
    const mine = ++token;
    try {
      const blob = await renderShareCard(content);
      if (mine !== token) return;

      const rendered = shareCardFile(blob, filename);
      objectUrl = URL.createObjectURL(rendered);
      file = rendered;
      previewUrl = objectUrl;
      status = "ready";
    } catch {
      if (mine === token) status = "failed";
    }
  }

  $effect(() => {
    if (open) void render();
    else discard();
  });

  // Only the URL, and only because the browser holds it until told otherwise —
  // it is the one thing here that outlives the component. Resetting the
  // reactive state on the way out would be writing to state nothing will read
  // again. Bumping the token first stops an in-flight render from doing the
  // same.
  onDestroy(() => {
    token++;
    revoke();
  });

  // Asked of the real file, because that is what the browser inspects: a
  // desktop that cannot take a file share gets a Download button that says so
  // rather than a Share button that quietly saves.
  const action = $derived(file && canShareFile(file) ? "Share" : "Download");

  async function send() {
    if (!file) return;
    busy = true;
    problem = "";
    try {
      // A dismissed share sheet comes back as "cancelled" — the dialog stays up
      // so the image is still there to try again.
      if ((await shareOrDownload(file)) !== "cancelled") open = false;
    } catch {
      problem = "Couldn't share the image.";
    } finally {
      busy = false;
    }
  }
</script>

<AlertDialog.Root bind:open>
  <AlertDialog.Content class="sm:max-w-md">
    <AlertDialog.Header>
      <AlertDialog.Title>{title}</AlertDialog.Title>
      <AlertDialog.Description>{subtitle}</AlertDialog.Description>
    </AlertDialog.Header>

    {#if status === "rendering"}
      <div
        class="mx-auto w-full max-w-[280px] animate-pulse rounded-lg bg-muted"
        style="aspect-ratio: {SHARE_CARD.width} / {SHARE_CARD.height}"
        role="status"
      >
        <span class="sr-only">Drawing your card…</span>
      </div>
    {:else if status === "failed"}
      <p class="text-center text-sm text-muted-foreground" role="alert">
        Couldn't draw the card.
      </p>
    {:else}
      <img
        src={previewUrl}
        {alt}
        width={SHARE_CARD.width}
        height={SHARE_CARD.height}
        class="mx-auto max-h-[55vh] w-auto rounded-lg ring-1 ring-foreground/10"
      />
    {/if}

    {#if problem}
      <p class="text-center text-sm text-destructive" role="alert">{problem}</p>
    {/if}

    <AlertDialog.Footer>
      <AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
      <!-- A plain Button rather than AlertDialog.Action: Action closes the
           dialog on click, which would take the preview away before a failed
           share had anywhere to report itself. -->
      <Button onclick={send} disabled={status !== "ready" || busy}>
        {busy ? "Working…" : action}
      </Button>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
