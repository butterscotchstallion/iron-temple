<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import Download from "@lucide/svelte/icons/download";
  import ErrorBanner from "../ErrorBanner.svelte";
  import { downloadAccountExport } from "../accountExport";

  let exporting = $state(false);
  let exportError = $state<string | null>(null);

  // Hand over the whole account as a file. A failure has to be said out loud:
  // the one thing worse than not having a backup is believing you do.
  async function exportAccount() {
    if (exporting) return;
    exporting = true;
    exportError = null;
    try {
      await downloadAccountExport();
    } catch {
      exportError = "Couldn't build your export. Try again in a moment.";
    } finally {
      exporting = false;
    }
  }
</script>

<Card class="p-6">
  <h3 class="text-lg font-bold text-card-foreground">Your data</h3>
  <p class="mt-1 text-sm text-muted-foreground">
    Every session, set and setting on this account, as one JSON file. Yours to
    keep, and readable without this app.
  </p>
  <div class="mt-4 flex flex-col gap-3">
    {#if exportError}
      <ErrorBanner
        message={exportError}
        onDismiss={() => (exportError = null)}
      />
    {/if}
    <div>
      <Button type="button" onclick={exportAccount} disabled={exporting}>
        <Download class="size-4" aria-hidden="true" />
        {exporting ? "Preparing…" : "Download my data"}
      </Button>
    </div>
  </div>
</Card>
