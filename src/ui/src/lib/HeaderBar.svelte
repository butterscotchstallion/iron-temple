<script lang="ts">
  import { onMount } from "svelte";
  import { getHealth } from "./api";
  import UserMenu from "./UserMenu.svelte";

  // The black bar across the top: build version on the left, account on the
  // right. Full-bleed and sticky, so it sits outside <main>'s centred column.

  // Version + environment come from the API's /health (single source of truth),
  // so this reflects the running backend, not a build-time constant. Moved here
  // from the footer, where it was easy to miss.
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

<header
  class="sticky top-0 z-40 w-full border-b border-white/10 bg-black/95 backdrop-blur"
>
  <div class="mx-auto flex h-12 max-w-5xl items-center justify-between gap-4 px-5">
    <span
      class="truncate text-[0.65rem] uppercase tracking-[0.25em] text-ink/50"
      data-testid="version"
    >
      {#if version}
        iron-temple {version}{#if environment}-{environment}{/if}
      {/if}
    </span>

    <UserMenu />
  </div>
</header>
