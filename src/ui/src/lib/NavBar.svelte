<script lang="ts">
  import { link } from "svelte-spa-router";
  import { onMount } from "svelte";

  const items = [
    { href: "/", label: "Workout" },
    { href: "/history", label: "History" },
    { href: "/progress", label: "Progress" },
  ];

  // svelte-spa-router@5.1.1 does not export a `location` store (importing it fails the
  // build). The router is hash-based, so read the current route from the URL hash
  // directly — routes look like "#/history", and `hashchange` fires on every
  // navigation, including use:link clicks.
  function currentPath(): string {
    return window.location.hash.replace(/^#/, "") || "/";
  }

  let path = $state(currentPath());

  onMount(() => {
    const update = () => (path = currentPath());
    window.addEventListener("hashchange", update);
    return () => window.removeEventListener("hashchange", update);
  });

  // A tab owns its section, including nested detail routes. "Workout" covers
  // the program/session flows that hang off the home tab.
  function isActive(href: string, p: string): boolean {
    if (href === "/") {
      return (
        p === "/" || p.startsWith("/programs") || p.startsWith("/sessions")
      );
    }
    return p === href || p.startsWith(href + "/");
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
        aria-current={isActive(item.href, path) ? "page" : undefined}
        class="rounded-full px-4 py-1.5 text-xs font-semibold uppercase tracking-[0.2em] transition {isActive(
          item.href,
          path,
        )
          ? 'bg-primary text-primary-foreground shadow-[0_0_14px_rgba(176,38,255,0.6)]'
          : 'text-muted-foreground hover:bg-foreground/5 hover:text-foreground'}"
      >
        {item.label}
      </a>
    {/each}
  </div>
</nav>
