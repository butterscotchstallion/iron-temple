<script lang="ts">
  import { link } from "svelte-spa-router";
  import { PROFILE_SECTIONS, type ProfileSection } from "./sections";

  // The profile's second row of navigation. Styled as NavBar's pills on
  // purpose — it is the same gesture one level down — but without the icons:
  // this sits directly under a row that has them, and two icon rows stacked
  // read as one crowded bar rather than as a hierarchy.

  let { current }: { current: ProfileSection } = $props();
</script>

<!-- aria-label, because this is the second <nav> on the page and "navigation"
     twice tells a screen-reader user nothing about which is which. -->
<nav aria-label="Profile sections" class="flex justify-center">
  <div
    class="inline-flex flex-wrap items-center justify-center gap-1 rounded-full border border-border/60 bg-card/40 p-1 backdrop-blur"
  >
    {#each PROFILE_SECTIONS as section (section.slug)}
      <a
        href="/profile/{section.slug}"
        use:link
        aria-current={section.slug === current ? "page" : undefined}
        class="rounded-full px-4 py-1.5 text-xs font-semibold uppercase tracking-[0.2em] transition {section.slug ===
        current
          ? 'bg-primary text-primary-foreground shadow-[0_0_14px_rgba(176,38,255,0.6)]'
          : 'text-muted-foreground hover:bg-foreground/5 hover:text-foreground'}"
      >
        {section.label}
      </a>
    {/each}
  </div>
</nav>
