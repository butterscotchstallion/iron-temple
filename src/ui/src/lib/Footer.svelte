<script lang="ts">
  import { onMount } from "svelte";
  import { getHealth } from "./api";

  // Version + environment come from the API's /health (single source of truth),
  // so the footer always reflects the running backend, not a build-time constant.
  let version = $state("");
  let environment = $state("");

  onMount(async () => {
    try {
      const { data } = await getHealth();
      version = data?.version ?? "";
      environment = data?.environment ?? "";
    } catch {
      // Cosmetic only — if /health is unreachable, just render nothing.
    }
  });
</script>

{#if version || environment}
  <footer
    class="mt-4 text-center text-xs uppercase tracking-[0.2em] text-muted-foreground/60"
  >
    iron-temple{#if version}
      {version}{/if}{#if environment}
      · {environment}{/if}
  </footer>
{/if}
