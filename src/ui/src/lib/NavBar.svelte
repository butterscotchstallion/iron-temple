<script lang="ts">
  import { link, location } from "svelte-spa-router";

  const items = [
    { href: "/", label: "Workout" },
    { href: "/history", label: "History" },
    { href: "/progress", label: "Progress" },
  ];

  // A tab owns its section, including nested detail routes. "Workout" covers
  // the program/session flows that hang off the home tab.
  function isActive(href: string, path: string): boolean {
    if (href === "/") {
      return (
        path === "/" ||
        path.startsWith("/programs") ||
        path.startsWith("/sessions")
      );
    }
    return path === href || path.startsWith(href + "/");
  }
</script>

<nav class="mt-5 flex justify-center">
  <div
    class="inline-flex items-center gap-1 rounded-full border border-border/60 bg-card/40 p-1 backdrop-blur"
  >
    {#each items as item (item.href)}
      <a
        href={item.href}
        use:link
        aria-current={isActive(item.href, $location) ? "page" : undefined}
        class="rounded-full px-4 py-1.5 text-xs font-semibold uppercase tracking-[0.2em] transition {isActive(
          item.href,
          $location,
        )
          ? 'bg-primary text-primary-foreground shadow-[0_0_14px_rgba(176,38,255,0.6)]'
          : 'text-muted-foreground hover:bg-foreground/5 hover:text-foreground'}"
      >
        {item.label}
      </a>
    {/each}
  </div>
</nav>
